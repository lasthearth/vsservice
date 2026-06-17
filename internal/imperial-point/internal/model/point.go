package model

import "time"

type PointControl struct {
	Side            string
	SettlementId    string
	ControlledSince time.Time
}

type ImperialPoint struct {
	Id            string
	Name          string
	Description   string
	BiRatePerHour int64
	TreeId        string
	Control       *PointControl // nil = unclaimed
}

// SetId sets the point's identifier (used after persistence).
func (p *ImperialPoint) SetId(id string) {
	p.Id = id
}

// RestoreControl sets the control state from persisted data (preserves original ControlledSince).
func (p *ImperialPoint) RestoreControl(ctrl *PointControl) {
	p.Control = ctrl
}

// SetControl updates the controlling settlement. Returns the previous side (empty if unclaimed).
func (p *ImperialPoint) SetControl(side, settlementId string) string {
	prev := ""
	if p.Control != nil {
		prev = p.Control.Side
	}
	p.Control = &PointControl{
		Side:            side,
		SettlementId:    settlementId,
		ControlledSince: time.Now(),
	}
	return prev
}

// ReleaseControl clears the controlling settlement. Returns the side that was released.
func (p *ImperialPoint) ReleaseControl() string {
	if p.Control == nil {
		return ""
	}
	side := p.Control.Side
	p.Control = nil
	return side
}
