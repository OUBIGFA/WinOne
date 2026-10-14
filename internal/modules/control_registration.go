package modules

import (
	"errors"
	"winone/internal/catalog"
	"winone/internal/host"
)

// RegisterPackage publishes a verified package's lifecycle and route together.
// The caller must use this Control's Host service and a gate bound to its live
// state store. No worker is started and persisted enable intent is not changed.
func (c *Control) RegisterPackage(service *host.Service, manifest catalog.Manifest, gate *Gate) error {
	if service == nil || gate == nil || gate.store != c.store || manifest.Distribution != "optional" || manifest.ID != gate.id || gate.packageSHA256 == "" || gate.unloaded.Load() {
		return errors.New("package runtime binding is incomplete")
	}
	if err := manifest.Validate(); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.gates[manifest.ID] != nil && (c.gates[manifest.ID] != gate || c.registrations[manifest.ID] != nil) {
		return errors.New("package runtime is already registered")
	}
	if !gate.matchesPackage(c.store.Snapshot().Modules[manifest.ID]) {
		return errors.New("package runtime binding changed")
	}
	// Service registration validates namespaces before mutating its catalog.
	// Holding the control lock keeps a concurrent enable from observing only
	// one half of this registration. A rejection leaves both maps unchanged.
	registration, err := service.BindFeature(manifest, gate.Status, gate.Handle)
	if err != nil {
		return err
	}
	c.gates[manifest.ID] = gate
	c.registrations[manifest.ID] = registration
	delete(c.unavailable, manifest.ID)
	return nil
}

// UnregisterPackage removes only this stopped gate's registration. Configuration
// and package files remain for the installation transaction to handle separately.
func (c *Control) UnregisterPackage(id string, gate *Gate) error {
	if gate == nil || gate.store != c.store || gate.id != id || gate.packageSHA256 == "" {
		return errors.New("package runtime binding is incomplete")
	}
	c.mu.Lock()
	if gate.unloaded.Load() && c.gates[id] != gate {
		c.mu.Unlock()
		return nil
	}
	registration := c.registrations[id]
	if c.gates[id] != gate || registration == nil {
		c.mu.Unlock()
		return errors.New("package runtime registration changed")
	}
	if c.active[id] {
		c.mu.Unlock()
		return errors.New("package lifecycle operation is still running")
	}
	// Keep status queries responsive while retirement finishes resource checks.
	// Admission stays closed for this module until both bindings are removed.
	c.active[id] = true
	c.mu.Unlock()
	err := gate.retire(registration.Remove)
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.active, id)
	if err != nil {
		return err
	}
	delete(c.gates, id)
	delete(c.registrations, id)
	return nil
}
