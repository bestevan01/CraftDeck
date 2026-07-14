package api

import (
	"context"
	"log"

	"craftdeck/internal/instance"
)

// ReconcileGamePorts implements the operator's "외부 접속 허용" extension:
// one toggle (network.Settings.WANEnabled) now covers both the web UI port
// and every directly-reachable Minecraft game port, and each instance's
// own port-forwarding rule follows that instance's start/stop lifecycle
// automatically rather than needing a separate manual step per server.
//
// "Directly reachable" means: the singleton Velocity proxy (its port is
// the front door for every server sitting behind it), or a server instance
// that isn't registered behind the proxy (independent exposure -- see
// ReconcileProxyMode/handleRegisterBehindProxy). A server sitting behind
// the proxy is bound to 127.0.0.1 only, so mapping its game_port would be
// pointless (nothing is actually listening on the public interface for
// it).
//
// Called after: the WAN toggle changes (handlers_network.go), an instance
// starts or stops (startInstanceCore/stopInstanceCore), a server's
// proxy-registration state changes (handleRegisterBehindProxy/
// handleUnregisterFromProxy, ReconcileProxyMode), and once at daemon
// startup (cmd/craftdeckd/main.go) to correct any drift from a prior run.
func (s *Server) ReconcileGamePorts(ctx context.Context) error {
	settings, err := s.networkSettings.Get(ctx)
	if err != nil {
		return err
	}

	list, err := s.instances.List(ctx)
	if err != nil {
		return err
	}

	for _, inst := range list {
		directlyExposed := inst.Kind == instance.KindProxy
		if inst.Kind == instance.KindServer {
			_, registered, err := s.serverSubdomain(ctx, inst.ID)
			if err != nil {
				return err
			}
			directlyExposed = !registered
		}
		running := inst.Status == instance.StatusRunning || inst.Status == instance.StatusStarting
		shouldMap := settings.WANEnabled && directlyExposed && running

		// sql.ErrNoRows (via scanMapping) is the normal case -- most
		// instances have no game-port mapping most of the time.
		existing, _ := s.portMappings.GetByInstance(ctx, inst.ID)

		switch {
		case shouldMap && existing == nil:
			id := inst.ID
			if _, _, err := s.netManager.Ensure(ctx, &id, inst.GamePort, "tcp", inst.Name); err != nil {
				log.Printf("reconcile game ports: map %s (%s): %v (continuing with the rest)", inst.Name, inst.ID, err)
			}
		case !shouldMap && existing != nil:
			if err := s.netManager.Remove(ctx, existing); err != nil {
				log.Printf("reconcile game ports: unmap %s (%s): %v (continuing with the rest)", inst.Name, inst.ID, err)
			}
		}
	}
	return nil
}

// removeGamePortMapping tears down instanceID's own game-port mapping, if
// it has one -- required before deleting the instance's row at all, since
// port_mappings.instance_id is a real foreign key on instances(id) with no
// ON DELETE CASCADE (same class of issue handleDeleteInstance already
// works around for plugins/backups). Confirmed on real hardware: deleting
// a proxy/server instance that still had an active mapping failed with
// "FOREIGN KEY constraint failed" until this was called first.
func (s *Server) removeGamePortMapping(ctx context.Context, instanceID string) error {
	mapping, _ := s.portMappings.GetByInstance(ctx, instanceID) // sql.ErrNoRows is the normal case
	if mapping == nil {
		return nil
	}
	return s.netManager.Remove(ctx, mapping)
}
