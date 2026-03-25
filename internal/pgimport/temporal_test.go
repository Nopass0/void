package pgimport

import (
	"testing"

	"github.com/voiddb/void/internal/engine"
)

func TestNormalizeColumnValueTimestampWithoutTimezone(t *testing.T) {
	got, err := NormalizeColumnValue("timestamp without time zone", "timestamp", "2026-03-25T10:11:12.123456")
	if err != nil {
		t.Fatalf("NormalizeColumnValue returned error: %v", err)
	}
	want := "2026-03-25T10:11:12.123456Z"
	if got != want {
		t.Fatalf("NormalizeColumnValue = %v, want %v", got, want)
	}
}

func TestNormalizeColumnValueDate(t *testing.T) {
	got, err := NormalizeColumnValue("date", "date", "2026-03-25")
	if err != nil {
		t.Fatalf("NormalizeColumnValue returned error: %v", err)
	}
	want := "2026-03-25T00:00:00Z"
	if got != want {
		t.Fatalf("NormalizeColumnValue = %v, want %v", got, want)
	}
}

func TestMapScalarTypeTimeUsesString(t *testing.T) {
	gotType, gotPrisma := MapScalarType("time without time zone", "time")
	if gotType != engine.TypeString || gotPrisma != "String" {
		t.Fatalf("MapScalarType = (%s, %s), want (%s, %s)", gotType, gotPrisma, engine.TypeString, "String")
	}
}
