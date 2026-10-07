package web

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// goldenFile holds the characterization of the demo database's costs: what
// the Abrechnung, the Dashboard and the Fixkosten list compute today. It is
// rewritten only with UPDATE_GOLDEN=1, so any change of an amount shows up
// as a failing test and a reviewable diff.
const goldenFile = "testdata/kosten_demo.golden"

var timeType = reflect.TypeOf(time.Time{})

// dumpValue writes v as stable text: struct fields in declaration order,
// map keys sorted, every float with two decimals (cents), dates as
// YYYY-MM-DD. Unexported fields are included, only reading is needed.
func dumpValue(sb *strings.Builder, name string, v reflect.Value, depth int) {
	indent := strings.Repeat("  ", depth)
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if v.IsNil() {
			fmt.Fprintf(sb, "%s%s: nil\n", indent, name)
			return
		}
		dumpValue(sb, name, v.Elem(), depth)
	case reflect.Float32, reflect.Float64:
		fmt.Fprintf(sb, "%s%s: %.2f\n", indent, name, v.Float())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		fmt.Fprintf(sb, "%s%s: %d\n", indent, name, v.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		fmt.Fprintf(sb, "%s%s: %d\n", indent, name, v.Uint())
	case reflect.Bool:
		fmt.Fprintf(sb, "%s%s: %t\n", indent, name, v.Bool())
	case reflect.String:
		fmt.Fprintf(sb, "%s%s: %q\n", indent, name, v.String())
	case reflect.Struct:
		if v.Type() == timeType {
			// time.Time has only unexported fields, so copy it to an
			// addressable value and read it through its method.
			c := reflect.New(timeType).Elem()
			c.Set(v)
			fmt.Fprintf(sb, "%s%s: %s\n", indent, name, c.Interface().(time.Time).Format("2006-01-02"))
			return
		}
		fmt.Fprintf(sb, "%s%s:\n", indent, name)
		for i := 0; i < v.NumField(); i++ {
			dumpValue(sb, v.Type().Field(i).Name, v.Field(i), depth+1)
		}
	case reflect.Slice, reflect.Array:
		fmt.Fprintf(sb, "%s%s: [%d]\n", indent, name, v.Len())
		for i := 0; i < v.Len(); i++ {
			dumpValue(sb, fmt.Sprintf("[%d]", i), v.Index(i), depth+1)
		}
	case reflect.Map:
		keys := v.MapKeys()
		sort.Slice(keys, func(i, j int) bool { return fmt.Sprint(keys[i]) < fmt.Sprint(keys[j]) })
		fmt.Fprintf(sb, "%s%s: {%d}\n", indent, name, v.Len())
		for _, k := range keys {
			dumpValue(sb, fmt.Sprint(k), v.MapIndex(k), depth+1)
		}
	default:
		panic(fmt.Sprintf("dumpValue: unsupported kind %s at %s", v.Kind(), name))
	}
}

// TestKostenDemodatenGolden characterizes the costs of the demo database
// (fixed now, no clock or network): berechneAbrechnung per year and
// apartment, loadDashboardData and the Fixkosten list data.
func TestKostenDemodatenGolden(t *testing.T) {
	db := openTestDB(t)
	if err := store.SeedDemoData(db, time.Date(2026, time.October, 15, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("SeedDemoData: %v", err)
	}

	var sb strings.Builder
	// 2023 is a Teiljahr, 2024 and 2025 are complete, 2026 has Mängel, 2022
	// has no data at all.
	for _, jahr := range []int{2022, 2023, 2024, 2025, 2026} {
		for _, apt := range []int64{1, 2} {
			erg, err := berechneAbrechnungDB(db, jahr, apt)
			if err != nil {
				t.Fatalf("berechneAbrechnung(%d, %d): %v", jahr, apt, err)
			}
			dumpValue(&sb, fmt.Sprintf("Abrechnung %d Wohnung %d", jahr, apt), reflect.ValueOf(&erg).Elem(), 0)
		}
	}

	dash, err := loadDashboardData(db)
	if err != nil {
		t.Fatalf("loadDashboardData: %v", err)
	}
	dumpValue(&sb, "Dashboard", reflect.ValueOf(&dash).Elem(), 0)

	liste, err := alleFixkostenKosten(db)
	if err != nil {
		t.Fatalf("alleFixkostenKosten: %v", err)
	}
	dumpValue(&sb, "Fixkosten-Liste", reflect.ValueOf(&liste).Elem(), 0)

	got := sb.String()
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll(filepath.Dir(goldenFile), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(goldenFile, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(goldenFile)
	if err != nil {
		t.Fatalf("read golden file (create it with UPDATE_GOLDEN=1): %v", err)
	}
	if got != string(want) {
		gl, wl := strings.Split(got, "\n"), strings.Split(string(want), "\n")
		for i := 0; i < len(gl) && i < len(wl); i++ {
			if gl[i] != wl[i] {
				t.Fatalf("golden mismatch at line %d:\n got: %s\nwant: %s\n(rerun with UPDATE_GOLDEN=1 if the change is intended)", i+1, gl[i], wl[i])
			}
		}
		t.Fatalf("golden mismatch: got %d lines, want %d (rerun with UPDATE_GOLDEN=1 if intended)", len(gl), len(wl))
	}
}
