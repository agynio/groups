package server

import (
	"context"
	"log"
	"time"
)

type Reconciler struct {
	server   *Server
	interval time.Duration
}

func NewReconciler(server *Server, interval time.Duration) Reconciler {
	if interval <= 0 {
		panic("reconciliation interval must be positive")
	}
	return Reconciler{server: server, interval: interval}
}

func (r Reconciler) Run(ctx context.Context) {
	r.reconcileOnce(ctx)
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.reconcileOnce(ctx)
		}
	}
}

func (r Reconciler) reconcileOnce(ctx context.Context) {
	report, err := r.server.Reconcile(ctx)
	if err != nil {
		log.Printf("groups reconciliation failed: %v", err)
		return
	}
	if report.TuplesWritten == 0 && report.TuplesDeleted == 0 && report.OrphanedMembershipsRemoved == 0 {
		return
	}
	log.Printf(
		"groups reconciliation repaired drift groups=%d memberships=%d tuples_written=%d tuples_deleted=%d orphaned_memberships_removed=%d",
		report.GroupsScanned,
		report.MembershipsScanned,
		report.TuplesWritten,
		report.TuplesDeleted,
		report.OrphanedMembershipsRemoved,
	)
}
