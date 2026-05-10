package data

import (
	"crypto/rand"
	"fmt"
	mathrand "math/rand/v2"
	"regexp"
	"strconv"
	"strings"
)

var tokenRegex = regexp.MustCompile(`\$\{([^}]+)\}`)

// Generator evaluates template strings with variable and random function substitutions.
type Generator struct {
	variables map[string]string
}

// NewGenerator creates a Generator with the given variable map.
func NewGenerator(vars map[string]string) *Generator {
	if vars == nil {
		vars = make(map[string]string)
	}
	return &Generator{variables: vars}
}

// Generate replaces all ${...} tokens in tmpl with computed values.
func (g *Generator) Generate(tmpl string) string {
	return tokenRegex.ReplaceAllStringFunc(tmpl, func(match string) string {
		inner := match[2 : len(match)-1] // strip ${ and }
		return g.evaluate(strings.TrimSpace(inner))
	})
}

func (g *Generator) evaluate(token string) string {
	switch {
	case token == "random.uuid":
		return randomUUID()
	case token == "random.email":
		return randomEmail()
	case token == "random.bool":
		if mathrand.IntN(2) == 0 {
			return "true"
		}
		return "false"
	case strings.HasPrefix(token, "random.int("):
		return g.evalRandomInt(token)
	case strings.HasPrefix(token, "random.float("):
		return g.evalRandomFloat(token)
	case strings.HasPrefix(token, "random.string("):
		return g.evalRandomString(token)
	case strings.HasPrefix(token, "random.choice("):
		return g.evalRandomChoice(token)
	case strings.HasPrefix(token, "var."):
		key := token[4:]
		if val, ok := g.variables[key]; ok {
			return val
		}
		return match(token)
	default:
		if val, ok := g.variables[token]; ok {
			return val
		}
		return match(token)
	}
}

func match(token string) string {
	return "${" + token + "}"
}

func parseArgs(token, prefix string) (string, bool) {
	if !strings.HasPrefix(token, prefix) || !strings.HasSuffix(token, ")") {
		return "", false
	}
	inner := token[len(prefix) : len(token)-1]
	return inner, true
}

func (g *Generator) evalRandomInt(token string) string {
	args, ok := parseArgs(token, "random.int(")
	if !ok {
		return match(token)
	}
	parts := strings.SplitN(args, ",", 2)
	if len(parts) != 2 {
		return match(token)
	}
	min, err1 := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
	max, err2 := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
	if err1 != nil || err2 != nil || max < min {
		return match(token)
	}
	return strconv.FormatInt(min+mathrand.Int64N(max-min+1), 10)
}

func (g *Generator) evalRandomFloat(token string) string {
	args, ok := parseArgs(token, "random.float(")
	if !ok {
		return match(token)
	}
	parts := strings.SplitN(args, ",", 2)
	if len(parts) != 2 {
		return match(token)
	}
	min, err1 := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	max, err2 := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err1 != nil || err2 != nil || max < min {
		return match(token)
	}
	return fmt.Sprintf("%.4f", min+mathrand.Float64()*(max-min))
}

func (g *Generator) evalRandomString(token string) string {
	args, ok := parseArgs(token, "random.string(")
	if !ok {
		return match(token)
	}
	n, err := strconv.Atoi(strings.TrimSpace(args))
	if err != nil || n <= 0 {
		return match(token)
	}
	return randomAlphanumeric(n)
}

func (g *Generator) evalRandomChoice(token string) string {
	args, ok := parseArgs(token, "random.choice(")
	if !ok {
		return match(token)
	}
	choices := strings.Split(args, ",")
	if len(choices) == 0 {
		return match(token)
	}
	for i := range choices {
		choices[i] = strings.TrimSpace(choices[i])
	}
	return choices[mathrand.IntN(len(choices))]
}

func randomUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func randomEmail() string {
	words := []string{"alice", "bob", "carol", "dave", "eve", "frank", "grace", "hank"}
	domains := []string{"example.com", "test.org", "mail.net", "demo.io"}
	w := words[mathrand.IntN(len(words))]
	n := mathrand.IntN(9000) + 1000
	d := domains[mathrand.IntN(len(domains))]
	return fmt.Sprintf("%s%d@%s", w, n, d)
}

const alphanumeric = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

func randomAlphanumeric(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = alphanumeric[mathrand.IntN(len(alphanumeric))]
	}
	return string(b)
}

