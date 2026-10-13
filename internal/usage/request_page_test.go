package usage

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/yetone/magpie/internal/catalog"
	"github.com/yetone/magpie/internal/sessions"
)

func pageHome(t *testing.T) {
	sessionHome(t)
	sessions.Reset()
	catalog.Reset()
	t.Cleanup(func() { sessions.Reset(); catalog.Reset() })
	os.MkdirAll(filepath.Dir(catalog.CachePath()), 0700)
	os.WriteFile(catalog.CachePath(), []byte(`{"openai":{"id":"openai","models":{"m":{"id":"m","cost":{"input":2,"output":8,"cache_read":0.5,"cache_write":2.5}}}}}`), 0600)
}
func equalPage(t *testing.T, got, want RequestPage) {
	t.Helper()
	// Summation order changes without a global sort; tolerate float round-off.
	var normalize func(reflect.Value)
	normalize = func(v reflect.Value) {
		switch v.Kind() {
		case reflect.Pointer:
			if !v.IsNil() {
				normalize(v.Elem())
			}
		case reflect.Struct:
			for i := 0; i < v.NumField(); i++ {
				if v.Field(i).CanSet() {
					normalize(v.Field(i))
				}
			}
		case reflect.Slice:
			for i := 0; i < v.Len(); i++ {
				normalize(v.Index(i))
			}
		case reflect.Map:
			for _, k := range v.MapKeys() {
				x := reflect.New(v.Type().Elem()).Elem()
				x.Set(v.MapIndex(k))
				normalize(x)
				v.SetMapIndex(k, x)
			}
		case reflect.Float64:
			v.SetFloat(math.Round(v.Float()*1e9) / 1e9)
		}
	}
	normalize(reflect.ValueOf(&got).Elem())
	normalize(reflect.ValueOf(&want).Elem())
	a, _ := json.Marshal(got)
	b, _ := json.Marshal(want)
	if string(a) != string(b) {
		t.Fatalf("page differs\ngot %s\nwant %s", a, b)
	}
}
