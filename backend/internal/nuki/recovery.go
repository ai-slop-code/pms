package nuki

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var amarildoRecoveryManifest = struct {
	PropertyID  int64
	SmartlockID string
	StayID      int64
	CodeID      int64
	RemoteID    string
	Label       string
	Type        int64
	CreatedAt   time.Time
	ValidFrom   time.Time
	ValidUntil  time.Time
}{
	PropertyID:  1,
	SmartlockID: "18233733061",
	StayID:      293,
	CodeID:      238,
	RemoteID:    "6ab2be33a7fc3019addfa5d1",
	Label:       "Booking-Amarildo",
	Type:        13,
	CreatedAt:   time.Date(2026, 9, 22, 17, 43, 15, 0, time.UTC),
	ValidFrom:   time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC),
	ValidUntil:  time.Date(2026, 9, 23, 7, 0, 0, 0, time.UTC),
}

type AmarildoRecoveryResult struct {
	Applicable       bool   `json:"applicable"`
	AlreadyRecovered bool   `json:"already_recovered"`
	RemotePresent    bool   `json:"remote_present"`
	Message          string `json:"message"`
}

func (s *Service) PreviewAmarildoRecovery(ctx context.Context) (*AmarildoRecoveryResult, error) {
	return s.validateAmarildoRecovery(ctx)
}

// RecoverAmarildo restores only the manifest-backed PMS provenance. It never
// creates, enables, extends, or directly deletes a provider authorization.
// The normal expiry lifecycle owns the subsequent deletion attempt.
func (s *Service) RecoverAmarildo(ctx context.Context) (*AmarildoRecoveryResult, error) {
	result, err := s.validateAmarildoRecovery(ctx)
	if err != nil || !result.Applicable {
		return result, err
	}
	if result.AlreadyRecovered {
		return result, nil
	}
	code, err := s.Store.GetNukiCodeByID(ctx, amarildoRecoveryManifest.PropertyID, amarildoRecoveryManifest.CodeID)
	if err != nil || code == nil {
		return result, fmt.Errorf("amarildo local code unavailable")
	}
	if err := s.Store.ApplyNukiIncidentRecovery(ctx, amarildoRecoveryManifest.PropertyID, amarildoRecoveryManifest.StayID, amarildoRecoveryManifest.CodeID, amarildoRecoveryManifest.SmartlockID, amarildoRecoveryManifest.RemoteID, amarildoRecoveryManifest.Label, amarildoRecoveryManifest.ValidFrom, amarildoRecoveryManifest.ValidUntil, result.RemotePresent); err != nil {
		return result, err
	}
	if result.RemotePresent {
		result.Message = "ownership restored; deletion pending"
	} else {
		result.Message = "ownership restored; confirmed remote absence"
	}
	return result, nil
}

func (s *Service) validateAmarildoRecovery(ctx context.Context) (*AmarildoRecoveryResult, error) {
	result := &AmarildoRecoveryResult{}
	code, err := s.Store.GetNukiCodeByID(ctx, amarildoRecoveryManifest.PropertyID, amarildoRecoveryManifest.CodeID)
	if err != nil {
		return result, err
	}
	if code == nil || !code.NamedStayID.Valid || code.NamedStayID.Int64 != amarildoRecoveryManifest.StayID || code.CodeLabel != amarildoRecoveryManifest.Label {
		return result, errors.New("amarildo recovery manifest does not match local data")
	}
	if code.ValidFrom.UTC() != amarildoRecoveryManifest.ValidFrom || code.ValidUntil.UTC() != amarildoRecoveryManifest.ValidUntil {
		return result, errors.New("amarildo recovery manifest validity mismatch")
	}
	managed, err := s.Store.ManagedNukiCredentialForRemote(ctx, amarildoRecoveryManifest.PropertyID, amarildoRecoveryManifest.SmartlockID, amarildoRecoveryManifest.RemoteID)
	if err != nil {
		return result, err
	}
	if managed != nil && managed.NamedStayID.Valid && managed.NamedStayID.Int64 == amarildoRecoveryManifest.StayID && managed.NukiAccessCodeID.Valid && managed.NukiAccessCodeID.Int64 == amarildoRecoveryManifest.CodeID && managed.Provenance == "evidence_backed_incident_recovery" {
		result.Applicable, result.AlreadyRecovered = true, true
		result.Message = "manifest ownership already restored"
		return result, nil
	}
	_, _, cred, _, _, _, _, _, err := s.loadNukiSyncContext(ctx, amarildoRecoveryManifest.PropertyID, false)
	if err != nil {
		return result, err
	}
	if cred.SmartLockID != amarildoRecoveryManifest.SmartlockID {
		return result, errors.New("amarildo configured smartlock mismatch")
	}
	rows, err := s.Client.ListKeypadCodes(ctx, cred)
	if err != nil {
		return result, err
	}
	for _, row := range rows {
		if strings.TrimSpace(row.ExternalID) != amarildoRecoveryManifest.RemoteID {
			continue
		}
		if row.SmartlockID != amarildoRecoveryManifest.SmartlockID || row.Type != amarildoRecoveryManifest.Type || row.Name != amarildoRecoveryManifest.Label || row.ValidFrom == nil || row.ValidUntil == nil || !row.ValidFrom.UTC().Equal(amarildoRecoveryManifest.ValidFrom) || !row.ValidUntil.UTC().Equal(amarildoRecoveryManifest.ValidUntil) {
			return result, errors.New("amarildo remote tuple mismatch")
		}
		var payload map[string]interface{}
		if err := json.Unmarshal([]byte(row.PayloadJSON), &payload); err != nil {
			return result, errors.New("amarildo remote evidence malformed")
		}
		if n, ok := payload["type"].(float64); !ok || int64(n) != amarildoRecoveryManifest.Type {
			return result, errors.New("amarildo remote type mismatch")
		}
		created := parseAnyTime(payload["creationDate"])
		if created == nil || !created.UTC().Equal(amarildoRecoveryManifest.CreatedAt) {
			return result, errors.New("amarildo remote creation timestamp mismatch")
		}
		result.Applicable, result.RemotePresent = true, true
		result.Message = "exact remote tuple validated"
		return result, nil
	}
	result.Applicable = true
	result.Message = "exact remote identity absent; provenance can be restored without DELETE"
	return result, nil
}
