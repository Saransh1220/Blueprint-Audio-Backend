package admin

import "testing"

func TestModuleConstructionAndHandlerAccess(t *testing.T) {
	module := NewModule(nil, nil)
	if module == nil {
		t.Fatal("expected module")
	}
	if module.HTTPHandler() == nil {
		t.Fatal("expected HTTP handler")
	}
}
