package config

import (
	"reflect"
	"testing"
)

func TestConfigHasNoPMS21DualWriteFlags(t *testing.T) {
	typeOfConfig := reflect.TypeOf(Config{})
	for _, field := range []string{"RawBlocksDualWrite", "OccupancyLegacyWriteDisabled", "OccupancyExportDisabled"} {
		if _, ok := typeOfConfig.FieldByName(field); ok {
			t.Fatalf("legacy config field %s still exists", field)
		}
	}
}

func TestLoadIgnoresRemovedPMS21DualWriteEnvironment(t *testing.T) {
	t.Setenv("PMS_ENV", "test")
	t.Setenv("PMS21_RAW_BLOCKS_DUAL_WRITE", "true")
	t.Setenv("PMS21_OCCUPANCY_LEGACY_WRITE_DISABLED", "true")
	t.Setenv("PMS21_OCCUPANCY_EXPORT_DISABLED", "true")
	if _, err := Load(); err != nil {
		t.Fatal(err)
	}
}
