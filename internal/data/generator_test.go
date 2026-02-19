package data

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestGenerate_Variables(t *testing.T) {
	g := NewGenerator(map[string]string{
		"base_url": "http://localhost",
		"token":    "abc123",
	})

	result := g.Generate("${base_url}/path?t=${token}")
	if result != "http://localhost/path?t=abc123" {
		t.Errorf("unexpected result: %q", result)
	}
}

func TestGenerate_VarPrefix(t *testing.T) {
	g := NewGenerator(map[string]string{"mykey": "myval"})
	result := g.Generate("${var.mykey}")
	if result != "myval" {
		t.Errorf("unexpected result: %q", result)
	}
}

func TestGenerate_MissingVariable(t *testing.T) {
	g := NewGenerator(nil)
	result := g.Generate("${missing}")
	if result != "${missing}" {
		t.Errorf("expected passthrough for missing var, got %q", result)
	}
}

func TestGenerate_RandomUUID(t *testing.T) {
	g := NewGenerator(nil)
	uuidRe := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	for i := 0; i < 20; i++ {
		result := g.Generate("${random.uuid}")
		if !uuidRe.MatchString(result) {
			t.Errorf("invalid UUID: %q", result)
		}
	}
}

func TestGenerate_RandomEmail(t *testing.T) {
	g := NewGenerator(nil)
	emailRe := regexp.MustCompile(`^[a-z]+\d+@[a-z.]+$`)
	for i := 0; i < 10; i++ {
		result := g.Generate("${random.email}")
		if !emailRe.MatchString(result) {
			t.Errorf("invalid email: %q", result)
		}
	}
}

func TestGenerate_RandomBool(t *testing.T) {
	g := NewGenerator(nil)
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		v := g.Generate("${random.bool}")
		if v != "true" && v != "false" {
			t.Errorf("unexpected bool value: %q", v)
		}
		seen[v] = true
	}
	if !seen["true"] || !seen["false"] {
		t.Error("expected both true and false to appear in 100 iterations")
	}
}

func TestGenerate_RandomInt(t *testing.T) {
	g := NewGenerator(nil)
	for i := 0; i < 50; i++ {
		result := g.Generate("${random.int(1,100)}")
		n, err := strconv.Atoi(result)
		if err != nil {
			t.Fatalf("non-integer result: %q", result)
		}
		if n < 1 || n > 100 {
			t.Errorf("out of range: %d", n)
		}
	}
}

func TestGenerate_RandomFloat(t *testing.T) {
	g := NewGenerator(nil)
	for i := 0; i < 20; i++ {
		result := g.Generate("${random.float(0.0,1.0)}")
		f, err := strconv.ParseFloat(result, 64)
		if err != nil {
			t.Fatalf("non-float result: %q", result)
		}
		if f < 0.0 || f > 1.0 {
			t.Errorf("out of range: %f", f)
		}
	}
}

func TestGenerate_RandomString(t *testing.T) {
	g := NewGenerator(nil)
	result := g.Generate("${random.string(16)}")
	if len(result) != 16 {
		t.Errorf("expected length 16, got %d: %q", len(result), result)
	}
}

func TestGenerate_RandomChoice(t *testing.T) {
	g := NewGenerator(nil)
	choices := []string{"foo", "bar", "baz"}
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		result := g.Generate("${random.choice(foo,bar,baz)}")
		found := false
		for _, c := range choices {
			if result == c {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("unexpected choice: %q", result)
		}
		seen[result] = true
	}
	if len(seen) < 2 {
		t.Error("expected multiple choices to appear in 100 iterations")
	}
}

func TestGenerate_MultipleTokens(t *testing.T) {
	g := NewGenerator(map[string]string{"host": "example.com"})
	tmpl := `{"host":"${host}","id":"${random.uuid}","n":${random.int(1,9)}}`
	result := g.Generate(tmpl)
	if !strings.Contains(result, "example.com") {
		t.Errorf("missing host in result: %q", result)
	}
	if strings.Contains(result, "${") {
		t.Errorf("unresolved tokens in result: %q", result)
	}
}

func TestGenerate_NoTokens(t *testing.T) {
	g := NewGenerator(nil)
	plain := "no tokens here"
	if result := g.Generate(plain); result != plain {
		t.Errorf("expected passthrough, got %q", result)
	}
}
