package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// opRef is a 1Password item reference: "account/vault/item", "vault/item",
// or bare "item".
type opRef struct {
	Account string
	Vault   string
	Item    string
}

func parseOPRef(s string) (opRef, error) {
	parts := strings.Split(s, "/")
	for _, p := range parts {
		if p == "" {
			return opRef{}, fmt.Errorf("invalid 1Password reference %q", s)
		}
	}
	switch len(parts) {
	case 1:
		return opRef{Item: parts[0]}, nil
	case 2:
		return opRef{Vault: parts[0], Item: parts[1]}, nil
	case 3:
		return opRef{Account: parts[0], Vault: parts[1], Item: parts[2]}, nil
	default:
		return opRef{}, fmt.Errorf("invalid 1Password reference %q (expected [account/][vault/]item)", s)
	}
}

type opField struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Purpose string `json:"purpose"`
	Value   string `json:"value"`
}

// pickOPField selects the secret-bearing field of a 1Password item:
// the "credential" field (API Credential category), then the item's
// password field, by id, label, then purpose.
func pickOPField(fields []opField) (string, bool) {
	match := func(ok func(opField) bool) (string, bool) {
		for _, f := range fields {
			if f.Value != "" && ok(f) {
				return f.Value, true
			}
		}
		return "", false
	}
	if v, ok := match(func(f opField) bool { return f.ID == "credential" || f.Label == "credential" }); ok {
		return v, true
	}
	return match(func(f opField) bool { return f.Purpose == "PASSWORD" || f.ID == "password" || f.Label == "password" })
}

// opAPIKey resolves a 1Password reference to a secret via the op CLI.
// Fails fast when op is not installed or the item cannot be resolved.
func opAPIKey(ref string) (string, error) {
	op, err := exec.LookPath("op")
	if err != nil {
		return "", fmt.Errorf("1Password CLI (op) not found in PATH")
	}
	r, err := parseOPRef(ref)
	if err != nil {
		return "", err
	}
	args := []string{"item", "get", r.Item, "--format", "json"}
	if r.Vault != "" {
		args = append(args, "--vault", r.Vault)
	}
	if r.Account != "" {
		args = append(args, "--account", r.Account)
	}
	cmd := exec.Command(op, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("op item get %q: %s", ref, msg)
	}
	var item struct {
		Fields []opField `json:"fields"`
	}
	if err := json.Unmarshal(out, &item); err != nil {
		return "", fmt.Errorf("op item get %q: unexpected output: %w", ref, err)
	}
	secret, ok := pickOPField(item.Fields)
	if !ok {
		return "", fmt.Errorf("op item %q has no credential or password field", ref)
	}
	return secret, nil
}
