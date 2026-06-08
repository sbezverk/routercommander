package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetRoutersInventoryDefaultsAddressToRouterName(t *testing.T) {
	tmpDir := t.TempDir()
	inventoryFile := filepath.Join(tmpDir, "routers.yaml")
	inventoryYAML := `routers:
  DRCTAM51:
    platform: iosxr
    username: admin
  DRCTAM52: {}
  logical-r1:
    address: vxr-slurm-307
    port: 24965
`
	if err := os.WriteFile(inventoryFile, []byte(inventoryYAML), 0644); err != nil {
		t.Fatalf("failed to write inventory file: %v", err)
	}

	inventory, err := getRoutersInventory(inventoryFile)
	if err != nil {
		t.Fatalf("failed to load inventory: %v", err)
	}

	tests := []struct {
		name        string
		wantAddress string
		wantPort    int
	}{
		{name: "drctam51", wantAddress: "drctam51", wantPort: 22},
		{name: "drctam52", wantAddress: "drctam52", wantPort: 22},
		{name: "logical-r1", wantAddress: "vxr-slurm-307", wantPort: 24965},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			target, ok := inventory.Routers[tt.name]
			if !ok {
				t.Fatalf("expected router %q to be present", tt.name)
			}
			if target.Address != tt.wantAddress {
				t.Fatalf("address = %q, want %q", target.Address, tt.wantAddress)
			}
			if target.Port != tt.wantPort {
				t.Fatalf("port = %d, want %d", target.Port, tt.wantPort)
			}
		})
	}
}

func TestResolveRouterTargetDefaultsAddressToRouterNameAndUsername(t *testing.T) {
	inventory := &RouterInventory{
		Routers: map[string]*RouterTarget{
			"drctam51": {
				Platform: "iosxr",
			},
			"drctam52": {
				Username: "admin",
			},
		},
	}

	target, err := resolveRouterTarget("DRCTAM51", inventory, 2222, "cisco")
	if err != nil {
		t.Fatalf("failed to resolve router target: %v", err)
	}

	if target.Address != "drctam51" {
		t.Fatalf("address = %q, want %q", target.Address, "drctam51")
	}
	if target.Port != 2222 {
		t.Fatalf("port = %d, want %d", target.Port, 2222)
	}
	if target.Username != "cisco" {
		t.Fatalf("username = %q, want %q", target.Username, "cisco")
	}
	if inventory.Routers["drctam51"].Port != 0 {
		t.Fatalf("inventory port was mutated to %d, want 0", inventory.Routers["drctam51"].Port)
	}
	target, err = resolveRouterTarget("DRCTAM51", inventory, 2022, "cisco")
	if err != nil {
		t.Fatalf("failed to resolve router target: %v", err)
	}
	if target.Port != 2022 {
		t.Fatalf("port = %d, want %d", target.Port, 2022)
	}

	target, err = resolveRouterTarget("DRCTAM52", inventory, 2222, "cisco")
	if err != nil {
		t.Fatalf("failed to resolve router target: %v", err)
	}
	if target.Username != "admin" {
		t.Fatalf("username = %q, want %q", target.Username, "admin")
	}
}
