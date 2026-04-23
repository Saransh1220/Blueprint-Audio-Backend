# Go HTTP Routing: Handle vs HandleFunc & Middlewares

In Go's standard library (`net/http`), routing and handling HTTP requests revolve around a few core types and concepts. If you understand these, the entire Go web ecosystem makes complete sense. 

This guide breaks down these concepts using examples from our `Blueprint-Audio-Backend` and provides a practice set for you to experiment with outside of this project.

---

## 1. The Core Interface: `http.Handler`

Everything in Go's HTTP server is built around one extremely simple interface called `http.Handler`:

```go
type Handler interface {
    ServeHTTP(ResponseWriter, *Request)
}
```

Any object (struct, int, string, etc.) that implements this `ServeHTTP` method is a "Handler". The HTTP router (the multiplexer, or "mux") only knows how to deal with `http.Handler`s. 

### Why is this useful?
It allows you to attach state (like database connections or config) to a struct, and let that struct handle requests.

```go
// Example: A struct acting as a handler
type HelloHandler struct {
    Greeting string
}

func (h *HelloHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    w.Write([]byte(h.Greeting))
}
```

---

## 2. The Adapter: `http.HandlerFunc`

Writing a struct for every single route gets tedious. Most of the time, you just want to write a normal function. 

Go provides a built-in type adapter called `http.HandlerFunc`. (Notice the capital 'F' - it's a type, not a method).

```go
// This is how Go defines it internally:
type HandlerFunc func(ResponseWriter, *Request)

// And it gives it a ServeHTTP method that just calls itself:
func (f HandlerFunc) ServeHTTP(w ResponseWriter, r *Request) {
    f(w, r)
}
```

Because of this brilliant trick, **any function** with the signature `func(http.ResponseWriter, *http.Request)` can be instantly converted into an `http.Handler` interface simply by wrapping it: `http.HandlerFunc(myFunction)`.

---

## 3. `mux.Handle` vs `mux.HandleFunc`

The router (`http.ServeMux`) provides two methods to register routes. They do exactly the same thing under the hood; one is just a convenience helper.

### `mux.Handle(pattern string, handler http.Handler)`
This expects an object that implements the `http.Handler` interface.
If you pass a struct (like `HelloHandler` above) or a middleware chain that outputs a `Handler`, you use this.

### `mux.HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request))`
This is pure convenience. It expects a raw function. Inside the Go source code, `mux.HandleFunc` literally just wraps your function in `http.HandlerFunc()` and calls `mux.Handle()`.

---

## 4. Examples from Our Codebase

Let's look at `internal/gateway/routes.go` to see these in action.

### Scenario A: Pure Function (`mux.HandleFunc`)
```go
mux.HandleFunc("POST /register", config.AuthHandler.Register)
```
`config.AuthHandler.Register` is just a method with the signature `func(w http.ResponseWriter, r *http.Request)`. Since it perfectly matches a raw function signature, we use `HandleFunc`.

### Scenario B: Middleware using Interface (`mux.Handle`)
```go
mux.Handle("GET /me", config.AuthMiddleware.RequireAuth(http.HandlerFunc(config.AuthHandler.Me)))
```
Let's break this down from the inside out:
1. `config.AuthHandler.Me` is a raw function.
2. Our `RequireAuth` middleware expects an `http.Handler` interface as input. So we must convert `Me` to an interface using `http.HandlerFunc(...)`.
3. `RequireAuth` returns an `http.Handler` interface.
4. Because the final result is an `http.Handler` interface, we *must* use `mux.Handle`.

### Scenario C: Middleware using Functions (`mux.HandleFunc`)
In our rate limiting code:
```go
emailActionLimiter := middleware.RateLimitMiddleware(3, 15*time.Minute)

mux.HandleFunc("POST /auth/verify-email", emailActionLimiter(config.AuthHandler.VerifyEmail))
```
Unlike `RequireAuth`, our `RateLimitMiddleware` was written to take a raw function (`http.HandlerFunc`) and return a raw function (`http.HandlerFunc`). 
Because the final result is a raw function, we can use `mux.HandleFunc`.

*(Note: In the Go community, the style used in Scenario B is considered the "Standard" way to write middleware because it works with both structs and functions. Scenario C is slightly less flexible but works fine for simple function-only APIs).*

---

## 5. Practice Exercises (Do this in a separate folder)

To really grasp this, create a new empty directory anywhere on your computer, run `go mod init practice`, and create a `main.go` file. Try to implement the following:

### Setup
```go
package main

import (
	"fmt"
	"net/http"
)

func main() {
	mux := http.NewServeMux()
    // Register your routes here!
	
	fmt.Println("Server running on :8080")
	http.ListenAndServe(":8080", mux)
}
```

### Challenge 1: The Basics
1. Create a normal function `func ping(w http.ResponseWriter, r *http.Request)` that writes "pong" to the response.
2. Register it using `mux.HandleFunc("/ping", ping)`.
3. Run `go run main.go` and visit `http://localhost:8080/ping`.

### Challenge 2: The Interface
1. Create a struct type `type CounterHandler struct { count int }`.
2. Give it a method `func (c *CounterHandler) ServeHTTP(w http.ResponseWriter, r *http.Request)`. Have it increment `c.count` and write the current count to the response (Hint: `fmt.Fprintf(w, "Count: %d", c.count)`).
3. Initialize the struct: `counter := &CounterHandler{}`
4. Register it using `mux.Handle("/count", counter)`. (Notice you cannot use `HandleFunc` here!).

### Challenge 3: Standard Middleware
1. Create a standard middleware function that logs the URL path before passing the request along:
```go
func LoggerMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        fmt.Println("Requested URL:", r.URL.Path)
        next.ServeHTTP(w, r)
    })
}
```
2. Wrap your `/count` struct from Challenge 2 with this middleware.
   *Hint: `mux.Handle("/count", LoggerMiddleware(counter))`*
3. Wrap your `/ping` function from Challenge 1 with this middleware.
   *Hint: Because `ping` is a function, you must convert it first! `mux.Handle("/ping", LoggerMiddleware(http.HandlerFunc(ping)))`*

### Challenge 4: The Function Middleware
1. Write a middleware that requires a specific header `X-Secret: mysecret`, but write it so it takes and returns `http.HandlerFunc` (like our Rate Limiter).
```go
func SecretMiddleware(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        if r.Header.Get("X-Secret") != "mysecret" {
            http.Error(w, "Forbidden", http.StatusForbidden)
            return
        }
        next(w, r) // Because 'next' is a function, we just call it directly!
    }
}
```
2. Register your `/ping` route again, but wrapped in `SecretMiddleware`.
   *Hint: You can use `mux.HandleFunc` now! `mux.HandleFunc("/secure-ping", SecretMiddleware(ping))`*
