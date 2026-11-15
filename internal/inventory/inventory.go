// Package inventory holds the resources that discovery finds in one AWS account and Region.
//
// The inventory never holds a secret value.
// A secret-bearing attribute keeps its name, and its value is Redacted.
package inventory

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// Redacted is the value that replaces a secret value in an attribute.
const Redacted = "[REDACTED]"

// ManagedStatus tells if Terraform manages a resource.
type ManagedStatus string

// These are the only valid ManagedStatus values.
const (
	StatusManaged   ManagedStatus = "managed"
	StatusUnmanaged ManagedStatus = "unmanaged"
	StatusUnknown   ManagedStatus = "unknown"
)

// These errors tell why Decode refused a resource.
var (
	ErrMissingType   = errors.New("resource has no type")
	ErrMissingID     = errors.New("resource has no id")
	ErrInvalidStatus = errors.New("resource has an invalid managed status")
)

// Inventory is the list of resources in one AWS account and Region.
type Inventory struct {
	Resources []Resource `json:"resources"`
}

// Resource is one AWS resource that discovery found.
type Resource struct {
	Type       string            `json:"type"`
	ID         string            `json:"id"`
	ARN        string            `json:"arn,omitempty"`
	Account    string            `json:"account,omitempty"`
	Region     string            `json:"region,omitempty"`
	Managed    ManagedStatus     `json:"managed"`
	Tags       map[string]string `json:"tags,omitempty"`
	Attributes map[string]string `json:"attributes,omitempty"`
	Links      []Link            `json:"links,omitempty"`
}

// Link is a relation from one resource to a different resource.
type Link struct {
	Relation   string `json:"relation"`
	TargetType string `json:"target_type"`
	TargetID   string `json:"target_id"`
}

// Encode writes inv to w as JSON.
// A nil resource list becomes an empty JSON array.
func Encode(w io.Writer, inv Inventory) error {
	if inv.Resources == nil {
		inv.Resources = []Resource{}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(inv); err != nil {
		return fmt.Errorf("encode inventory: %w", err)
	}
	return nil
}

// Decode reads one inventory from r as JSON and validates each resource.
// When the JSON is not valid or a resource is not valid, Decode returns the zero Inventory and an error.
func Decode(r io.Reader) (Inventory, error) {
	dec := json.NewDecoder(r)
	var inv Inventory
	if err := dec.Decode(&inv); err != nil {
		return Inventory{}, fmt.Errorf("decode inventory: %w", err)
	}
	if _, err := dec.Token(); err != io.EOF {
		return Inventory{}, errors.New("decode inventory: unexpected data after the JSON value")
	}
	for i, res := range inv.Resources {
		if err := validate(res); err != nil {
			return Inventory{}, fmt.Errorf("decode inventory: resources[%d]: %w", i, err)
		}
	}
	return inv, nil
}

// validate returns an error when res has no type, no ID, or an invalid managed status.
func validate(res Resource) error {
	if res.Type == "" {
		return ErrMissingType
	}
	if res.ID == "" {
		return ErrMissingID
	}
	switch res.Managed {
	case StatusManaged, StatusUnmanaged, StatusUnknown:
		return nil
	default:
		return fmt.Errorf("%w: %q", ErrInvalidStatus, res.Managed)
	}
}
