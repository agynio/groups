package server

// Reconciliation behavior for Private Networks rollout:
//
// The Groups service is the source of truth for group membership. Consumers that
// materialize OpenZiti role attributes must reconcile by periodically re-reading
// Groups.ListMemberGroups or Groups.ListMemberGroupsBatch for the identities they
// own. That keeps OpenZiti mutation and reconciliation in Users, Apps, and the
// Agents Orchestrator, where identity lifecycle ownership already lives.
//
// OpenFGA tuple drift is handled synchronously in this service on every supported
// mutation path: group creation writes group org/admin tuples; membership add and
// remove write/delete member tuples; group deletion deletes org, admin, and member
// tuples before removing the database row. If a publish to NATS or Notifications
// fails after the durable mutation, the API still succeeds and downstream
// consumer reconciliation repairs missed OpenZiti updates from this service's
// persisted membership state.
