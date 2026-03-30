package inventory

import (
	"fmt"
	"os"
	"sync"

	"gopkg.in/yaml.v3"
)

type Device struct {
	Name  string `yaml:"name"  json:"name"`
	Owner string `yaml:"owner" json:"owner"`
	Type  string `yaml:"type"  json:"type"`
	MAC   string `yaml:"mac"   json:"mac"`
	Notes string `yaml:"notes" json:"notes"`
}

type Inventory struct {
	mu      sync.RWMutex
	devices map[string]Device
	path    string
}

func Load(path string) (*Inventory, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read inventory: %w", err)
	}

	var raw map[string]Device
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse inventory: %w", err)
	}

	return &Inventory{devices: raw, path: path}, nil
}

func (inv *Inventory) Lookup(ip string) (Device, bool) {
	inv.mu.RLock()
	defer inv.mu.RUnlock()
	d, ok := inv.devices[ip]
	return d, ok
}

func (inv *Inventory) AllDevices() map[string]Device {
	inv.mu.RLock()
	defer inv.mu.RUnlock()
	out := make(map[string]Device, len(inv.devices))
	for k, v := range inv.devices {
		out[k] = v
	}
	return out
}

func (inv *Inventory) Count() int {
	inv.mu.RLock()
	defer inv.mu.RUnlock()
	return len(inv.devices)
}

// UpdateDevice updates or adds a device in the inventory and persists to disk.
func (inv *Inventory) UpdateDevice(ip string, dev Device) error {
	inv.mu.Lock()
	defer inv.mu.Unlock()
	inv.devices[ip] = dev
	return inv.save()
}

// DeleteDevice removes a device from the inventory and persists to disk.
func (inv *Inventory) DeleteDevice(ip string) error {
	inv.mu.Lock()
	defer inv.mu.Unlock()
	delete(inv.devices, ip)
	return inv.save()
}

func (inv *Inventory) save() error {
	data, err := yaml.Marshal(inv.devices)
	if err != nil {
		return fmt.Errorf("marshal inventory: %w", err)
	}
	return os.WriteFile(inv.path, data, 0644)
}
