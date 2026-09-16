// Package bustest is the standing contract check for bus.Device implementations.
//
// It exists because of the failure spike 003 named: a partially-correct snapshot does not fail. It
// restores a machine that is plausible and subtly wrong, and every fault it causes afterwards
// reads as a firmware fault. There is no runtime symptom to notice, so the only place the gap can
// be caught is here, and it has to be caught for every device rather than for the ones whose
// author remembered.
//
// It sits beside the bus rather than inside each device's own tests for the same reason
// testing/fstest sits beside io/fs: the contract belongs to the interface, and eight devices
// writing eight versions of this check is eight chances to write it slightly wrong.
//
// # Why the field-coverage check is not optional
//
// The first version of this package checked only that a mutated device round-tripped, and it
// PASSED a device whose Snapshot deliberately omitted a field. The mutator happened to leave that
// field at its fresh value, so the forgotten field was never different on either side of the
// round trip and the check compared it against itself. That is the defect this project has
// shipped before - an instrument reporting clean while examining nothing - so a Check now has to
// prove its mutator drives every field before any round-trip result from it means anything.
package bustest

import (
	"bytes"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/platform/hexfmt"
)

// Check describes how to exercise one bus.Device implementation.
type Check struct {
	// New returns a freshly reset device. It is called several times and must return an
	// independent device each time.
	New func() bus.Device

	// Mutate drives the device into an interesting state. It must move EVERY field the device
	// has, including the ones no bus read can observe - a flash chip's unlock position, an I2C
	// transaction half-finished, a link's partly-shifted byte. Those are the fields that get
	// forgotten, because forgetting them costs nothing until a restored machine behaves
	// differently for no visible reason. A field Mutate leaves alone is a field this check
	// cannot see, and it is reported rather than skipped.
	Mutate func(bus.Device)

	// Disturb moves a mutated device to a DIFFERENT state. It is what a restore has to erase,
	// and it has the same obligation to reach every field.
	Disturb func(bus.Device)

	// Constant names fields that are configuration rather than state - a memory's size, a
	// chip's part number - which Mutate and Disturb are not expected to move. Names are the
	// paths this package reports, so a nested field is "outer.inner". Keep the list short and
	// keep it justified: every name on it is a field this check stops examining.
	Constant []string
}

// CheckSnapshot proves that a device's Snapshot carries its complete state and that Restore
// replaces that state rather than merging into it.
//
// Four things are checked, and the first is what keeps the other three honest:
//
//  1. Coverage. Mutate must leave every field of the device different from a fresh one, and
//     Disturb must leave every field different again. A field neither of them moves is invisible
//     to the round trip below, which would then compare it against itself and pass.
//  2. Completeness. A snapshot of a mutated device, restored into a fresh one, must produce a
//     device equal to the original. A field Snapshot forgot stays at its fresh value here.
//  3. Replacement. Restoring that same snapshot onto a DISTURBED device must also produce a
//     device equal to the original. A field Restore rebuilds but never clears survives here.
//  4. That the snapshot has bytes in it at all.
//
// Devices are compared by their whole value, unexported fields included, so the check sees state
// no Read can reach. A device holding a channel, a function value or a logger will report a
// spurious difference: a hardware model should hold none of those, and one that genuinely must
// should keep it out of the device struct.
func CheckSnapshot(c Check) error {
	if c.New == nil || c.Mutate == nil || c.Disturb == nil {
		return fmt.Errorf("bustest: Check needs New, Mutate and Disturb")
	}

	fresh := c.New()
	if fresh == nil {
		return fmt.Errorf("bustest: New returned nil")
	}
	name := fresh.Name()

	constant := make(map[string]bool, len(c.Constant))
	for _, f := range c.Constant {
		constant[f] = true
	}
	if unknown := unknownConstants(fresh, constant); len(unknown) > 0 {
		return fmt.Errorf("bustest: harness failure: %s: Check.Constant names %s, which %s has no "+
			"such field for - a stale exemption silently stops checking a field that does exist",
			name, strings.Join(unknown, ", "), name)
	}

	// 1. Coverage, before anything that depends on it.
	want := c.New()
	c.Mutate(want)
	if missed := unmoved(fresh, want, constant); len(missed) > 0 {
		return fmt.Errorf("bustest: harness failure: %s: Mutate left %s at the value a fresh "+
			"device has, so this check cannot see %s and would pass a Snapshot that forgot %s",
			name, strings.Join(missed, ", "), plural(missed, "it", "them"), plural(missed, "it", "them"))
	}

	disturbed := c.New()
	c.Mutate(disturbed)
	c.Disturb(disturbed)
	if missed := unmoved(want, disturbed, constant); len(missed) > 0 {
		return fmt.Errorf("bustest: harness failure: %s: Disturb left %s unchanged from the "+
			"mutated device, so the replacement check cannot see whether Restore clears %s",
			name, strings.Join(missed, ", "), plural(missed, "it", "them"))
	}

	state, err := want.Snapshot()
	if err != nil {
		return fmt.Errorf("bustest: %s: Snapshot: %w", name, err)
	}
	if len(state) == 0 {
		return fmt.Errorf("bustest: harness failure: %s: Snapshot returned no bytes for a device "+
			"that is demonstrably in a non-default state", name)
	}

	// 2. Completeness.
	into := c.New()
	if err := into.Restore(state); err != nil {
		return fmt.Errorf("bustest: %s: Restore into a fresh device: %w", name, err)
	}
	if diff := difference(want, into); diff != "" {
		return fmt.Errorf("bustest: %s: Snapshot is incomplete - restoring it into a fresh device "+
			"did not reproduce the original: %s", name, diff)
	}

	// 3. Replacement.
	onto := c.New()
	c.Mutate(onto)
	c.Disturb(onto)
	if err := onto.Restore(state); err != nil {
		return fmt.Errorf("bustest: %s: Restore onto a running device: %w", name, err)
	}
	if diff := difference(want, onto); diff != "" {
		return fmt.Errorf("bustest: %s: Restore did not replace the device's state - something it "+
			"was doing before the restore survived it: %s", name, diff)
	}

	return nil
}

// unmoved returns the field paths that are identical in a and b, which for a mutator is the list
// of fields it failed to drive. Fields named in constant are left out.
func unmoved(a, b bus.Device, constant map[string]bool) []string {
	same := make([]string, 0, 4)
	for _, f := range leaves(a, b) {
		if constant[f.path] {
			continue
		}
		if walk(f.path, f.a, f.b) == "" {
			same = append(same, f.path)
		}
	}
	sort.Strings(same)
	return same
}

// unknownConstants returns the entries of constant that name no field of the device, so a field
// renamed out from under an exemption is reported rather than quietly stopping the check.
func unknownConstants(d bus.Device, constant map[string]bool) []string {
	known := make(map[string]bool)
	for _, f := range leaves(d, d) {
		known[f.path] = true
	}
	unknown := make([]string, 0, len(constant))
	for name := range constant {
		if !known[name] {
			unknown = append(unknown, name)
		}
	}
	sort.Strings(unknown)
	return unknown
}

type leaf struct {
	path string
	a, b reflect.Value
}

// leaves enumerates the comparable fields of two devices of the same type, recursing through
// structs and pointers but treating an array, slice or map as one field: a device's 32 MB of RAM
// is a field a mutator has to touch, not 33,554,432 of them.
func leaves(a, b bus.Device) []leaf {
	var out []leaf
	var rec func(path string, av, bv reflect.Value)
	rec = func(path string, av, bv reflect.Value) {
		if !av.IsValid() || !bv.IsValid() || av.Type() != bv.Type() {
			out = append(out, leaf{path, av, bv})
			return
		}
		// Every other kind is a leaf for this walk's purposes, which is what the default arm
		// says; enumerating the twenty-odd names would not make that any clearer.
		//exhaustive:ignore
		switch av.Kind() {
		case reflect.Pointer, reflect.Interface:
			if av.IsNil() || bv.IsNil() {
				out = append(out, leaf{path, av, bv})
				return
			}
			rec(path, addressable(av.Elem()), addressable(bv.Elem()))
		case reflect.Struct:
			for i := 0; i < av.NumField(); i++ {
				rec(join(path, av.Type().Field(i).Name), field(av, i), field(bv, i))
			}
		default:
			out = append(out, leaf{path, av, bv})
		}
	}
	rec("", addressable(reflect.ValueOf(a)), addressable(reflect.ValueOf(b)))
	return out
}

// difference returns a description of the first place a and b differ, naming the field path so a
// failure says which field was forgotten rather than only that one was. It returns "" when the two
// are equal.
func difference(a, b bus.Device) string {
	return walk("", addressable(reflect.ValueOf(a)), addressable(reflect.ValueOf(b)))
}

// addressable returns v in storage that has an address, copying it if it does not already have
// one. Unexported fields are read through their address below, and reflect.ValueOf's result has no
// address of its own; nor does a map element or the value inside an interface.
func addressable(v reflect.Value) reflect.Value {
	if !v.IsValid() || v.CanAddr() || !v.CanInterface() {
		return v
	}
	c := reflect.New(v.Type()).Elem()
	c.Set(v)
	return c
}

func walk(path string, a, b reflect.Value) string {
	if !a.IsValid() || !b.IsValid() {
		if a.IsValid() != b.IsValid() {
			return at(path, "one side is absent")
		}
		return ""
	}
	if a.Type() != b.Type() {
		return at(path, fmt.Sprintf("type %s vs %s", a.Type(), b.Type()))
	}

	// The arms below are the kinds that need structural handling; the default arm compares
	// everything else by value, so listing the remaining kinds would add nothing.
	//exhaustive:ignore
	switch a.Kind() {
	case reflect.Interface, reflect.Pointer:
		if a.IsNil() || b.IsNil() {
			if a.IsNil() != b.IsNil() {
				return at(path, fmt.Sprintf("%s vs %s", nilness(a), nilness(b)))
			}
			return ""
		}
		return walk(path, addressable(a.Elem()), addressable(b.Elem()))

	case reflect.Struct:
		for i := 0; i < a.NumField(); i++ {
			if d := walk(join(path, a.Type().Field(i).Name), field(a, i), field(b, i)); d != "" {
				return d
			}
		}
		return ""

	case reflect.Slice:
		if a.IsNil() != b.IsNil() {
			return at(path, fmt.Sprintf("%s vs %s", nilness(a), nilness(b)))
		}
		fallthrough
	case reflect.Array:
		if a.Len() != b.Len() {
			return at(path, fmt.Sprintf("length %d vs %d", a.Len(), b.Len()))
		}
		// A device's memory is a byte slice tens of megabytes long, and walking one element at a
		// time through reflect makes this check cost minutes. Compare the bytes directly and then
		// find the first difference the same way.
		if a.Type().Elem().Kind() == reflect.Uint8 && a.Kind() == reflect.Slice && a.CanInterface() {
			x, y := a.Bytes(), b.Bytes()
			if bytes.Equal(x, y) {
				return ""
			}
			for i := range x {
				if x[i] != y[i] {
					return at(fmt.Sprintf("%s[%d]", path, i),
						fmt.Sprintf("%s vs %s", hexfmt.Byte(x[i]), hexfmt.Byte(y[i])))
				}
			}
			return at(path, "bytes differ")
		}
		for i := 0; i < a.Len(); i++ {
			if d := walk(fmt.Sprintf("%s[%d]", path, i), a.Index(i), b.Index(i)); d != "" {
				return d
			}
		}
		return ""

	case reflect.Map:
		if a.Len() != b.Len() {
			return at(path, fmt.Sprintf("%d entries vs %d", a.Len(), b.Len()))
		}
		for _, k := range a.MapKeys() {
			bv := b.MapIndex(k)
			if !bv.IsValid() {
				return at(fmt.Sprintf("%s[%v]", path, k), "missing on one side")
			}
			if d := walk(fmt.Sprintf("%s[%v]", path, k), addressable(a.MapIndex(k)), addressable(bv)); d != "" {
				return d
			}
		}
		return ""

	default:
		if !reflect.DeepEqual(readable(a), readable(b)) {
			return at(path, fmt.Sprintf("%v vs %v", readable(a), readable(b)))
		}
		return ""
	}
}

// field reads struct field i, unexported fields included. reflect refuses Interface() on an
// unexported field, and unexported fields are precisely the state this check exists to see, so the
// field is read through the struct's own memory instead.
func field(v reflect.Value, i int) reflect.Value {
	f := v.Field(i)
	if f.CanInterface() || !v.CanAddr() {
		return f
	}
	return reflect.NewAt(f.Type(), f.Addr().UnsafePointer()).Elem()
}

func readable(v reflect.Value) any {
	if v.CanInterface() {
		return v.Interface()
	}
	return fmt.Sprintf("%v", v)
}

func nilness(v reflect.Value) string {
	if v.IsNil() {
		return "nil"
	}
	return "set"
}

func join(path, name string) string {
	if path == "" {
		return name
	}
	return path + "." + name
}

func at(path, what string) string {
	if path == "" {
		return what
	}
	return path + ": " + what
}

func plural(of []string, one, many string) string {
	if len(of) == 1 {
		return one
	}
	return many
}
