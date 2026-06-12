package main

import (
	"testing"
	"time"
)

func TestParseBusPath(t *testing.T) {
	cases := []struct {
		in    string
		busID string
		cam   int
		ok    bool
	}{
		{"bus_1_1", "1", 1, true},
		{"bus_42_3", "42", 3, true},
		{"live/bus_7_2", "7", 2, true},
		{"bus_1", "", 0, false},
		{"bus_1_x", "", 0, false},
		{"all_others", "", 0, false},
		{"bus__1", "", 0, false},
		{"bus_1_0", "", 0, false},  // cams are 1-based
		{"bus_1_99", "", 0, false}, // max 9 cams
	}
	for _, c := range cases {
		busID, cam, ok := parseBusPath(c.in)
		if busID != c.busID || cam != c.cam || ok != c.ok {
			t.Errorf("parseBusPath(%q) = (%q,%d,%v), want (%q,%d,%v)",
				c.in, busID, cam, ok, c.busID, c.cam, c.ok)
		}
	}
}

func TestBuildFleet(t *testing.T) {
	now := time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC)
	tracker := newFleetTracker()
	fleet := tracker.build([]mtxPath{
		{Name: "bus_1_1", Ready: true},
		{Name: "bus_1_2", Ready: true},
		{Name: "bus_2_1", Ready: true},
		{Name: "not_a_bus", Ready: true},
		{Name: "bus_3_1", Ready: false},
	}, now)

	if len(fleet.Buses) != 2 {
		t.Fatalf("got %d buses, want 2", len(fleet.Buses))
	}
	if fleet.Totals.BusesOnline != 2 || fleet.Totals.CamsOnline != 3 {
		t.Fatalf("totals = %+v, want 2 buses / 3 cams", fleet.Totals)
	}
	if fleet.Buses[0].ID != "1" || len(fleet.Buses[0].Cams) != 2 {
		t.Fatalf("bus[0] = %+v, want id 1 with 2 cams", fleet.Buses[0])
	}

	fleet2 := tracker.build([]mtxPath{
		{Name: "bus_2_1", Ready: true},
	}, now.Add(2*time.Minute))
	if len(fleet2.Buses) != 2 {
		t.Fatalf("got %d buses after dropout, want 2 (one recently-seen)", len(fleet2.Buses))
	}
	var bus1 *fleetBus
	for i := range fleet2.Buses {
		if fleet2.Buses[i].ID == "1" {
			bus1 = &fleet2.Buses[i]
		}
	}
	if bus1 == nil || len(bus1.Cams) != 0 {
		t.Fatalf("bus 1 should be present with 0 cams, got %+v", bus1)
	}

	fleet3 := tracker.build([]mtxPath{
		{Name: "bus_2_1", Ready: true},
	}, now.Add(11*time.Minute))
	if len(fleet3.Buses) != 1 {
		t.Fatalf("got %d buses after expiry, want 1", len(fleet3.Buses))
	}
}
