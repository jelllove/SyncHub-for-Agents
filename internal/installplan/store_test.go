package installplan

import "testing"

func TestApprovalIsBoundToAdapterAndSource(t *testing.T) {
	store := NewStore(t.TempDir())
	op := Operation{
		ID: "copilot:wiqd@wiqd", Adapter: "copilot-plugin",
		Source: "wiqd@wiqd", Executable: "copilot",
		Args: []string{"plugin", "install", "wiqd@wiqd"},
	}
	if err := store.Approve(op); err != nil {
		t.Fatal(err)
	}
	if !store.IsApproved(op) {
		t.Fatal("approved operation was not recognized")
	}
	op.Source = "wiqd@other-marketplace"
	if store.IsApproved(op) {
		t.Fatal("source change must require new approval")
	}
}

func TestPendingPlanRoundTripsAndClearsByID(t *testing.T) {
	store := NewStore(t.TempDir())
	plan := Plan{
		ID: "plan-1",
		Operations: []Operation{{
			ID: "op-1", Adapter: "copilot-plugin", Source: "wiqd@wiqd",
		}},
	}
	if err := store.SavePending(plan); err != nil {
		t.Fatal(err)
	}
	got, err := store.Pending()
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.ID != plan.ID || len(got.Operations) != 1 {
		t.Fatalf("pending plan = %#v", got)
	}
	if err := store.ClearPending("other"); err != nil {
		t.Fatal(err)
	}
	if got, _ := store.Pending(); got == nil {
		t.Fatal("mismatched plan ID cleared pending plan")
	}
	if err := store.ClearPending(plan.ID); err != nil {
		t.Fatal(err)
	}
	if got, err := store.Pending(); err != nil || got != nil {
		t.Fatalf("pending after clear = %#v, %v", got, err)
	}
}
