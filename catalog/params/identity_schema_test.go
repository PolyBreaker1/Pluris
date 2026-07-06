package params

import "testing"

func TestIdentitySchemaRegistered(t *testing.T) {
	schema := SchemaBySubtype("identity")
	if schema == nil {
		t.Fatal("expected identity schema to be registered")
	}
	if !schema.HasParam("display_name") {
		t.Fatal("expected identity schema to mount display_name")
	}
	if !schema.HasParam("tenant") || !schema.HasParam("site") {
		t.Fatal("expected identity schema to reuse the shared tenant/site params")
	}
	for _, key := range schema.DefaultColumns {
		if DefByKey(key) == nil {
			t.Fatalf("default column %q has no registered ParamDef", key)
		}
	}
}
