/*
Copyright 2014 Google Inc. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package greenrun

import (
	"fmt"
	"math/rand"
	"reflect"
	"time"
)

// greenrunFuncMap is a map from a type to a greenrunFunc that handles that type.
type greenrunFuncMap map[reflect.Type]reflect.Value

// GreenRunner knows how to fill any object with random fields.
type GreenRunner struct {
	greenrunFuncs        greenrunFuncMap
	defaultGreenRunFuncs greenrunFuncMap
	r                *rand.Rand
	nilChance        float64
	minElements      int
	maxElements      int
	maxDepth         int
}

// New returns a new GreenRunner. Customize your GreenRunner further by calling Funcs,
// RandSource, NilChance, or NumElements in any order.
func New() *GreenRunner {
	return NewWithSeed(time.Now().UnixNano())
}

func NewWithSeed(seed int64) *GreenRunner {
	f := &GreenRunner{
		defaultGreenRunFuncs: greenrunFuncMap{
			reflect.TypeOf(&time.Time{}): reflect.ValueOf(greenrunTime),
		},

		greenrunFuncs:   greenrunFuncMap{},
		r:           rand.New(rand.NewSource(seed)),
		nilChance:   .2,
		minElements: 1,
		maxElements: 10,
		maxDepth:    100,
	}
	return f
}

// Funcs adds each entry in greenrunFuncs as a custom greenruning function.
//
// Each entry in greenrunFuncs must be a function taking two parameters.
// The first parameter must be a pointer or map. It is the variable that
// function will fill with random data. The second parameter must be a
// greenrun.Continue, which will provide a source of randomness and a way
// to automatically continue greenruning smaller pieces of the first parameter.
//
// These functions are called sensibly, e.g., if you wanted custom string
// greenruning, the function `func(s *string, c greenrun.Continue)` would get
// called and passed the address of strings. Maps and pointers will always
// be made/new'd for you, ignoring the NilChange option. For slices, it
// doesn't make much sense to  pre-create them--GreenRunner doesn't know how
// long you want your slice--so take a pointer to a slice, and make it
// yourself. (If you don't want your map/pointer type pre-made, take a
// pointer to it, and make it yourself.) See the examples for a range of
// custom functions.
func (f *GreenRunner) Funcs(greenrunFuncs ...interface{}) *GreenRunner {
	for i := range greenrunFuncs {
		v := reflect.ValueOf(greenrunFuncs[i])
		if v.Kind() != reflect.Func {
			panic("Need only funcs!")
		}
		t := v.Type()
		if t.NumIn() != 2 || t.NumOut() != 0 {
			panic("Need 2 in and 0 out params!")
		}
		argT := t.In(0)
		switch argT.Kind() {
		case reflect.Ptr, reflect.Map:
		default:
			panic("greenrunFunc must take pointer or map type")
		}
		if t.In(1) != reflect.TypeOf(Continue{}) {
			panic("greenrunFunc's second parameter must be type greenrun.Continue")
		}
		f.greenrunFuncs[argT] = v
	}
	return f
}

// RandSource causes f to get values from the given source of randomness.
// Use if you want deterministic greenruning.
func (f *GreenRunner) RandSource(s rand.Source) *GreenRunner {
	f.r = rand.New(s)
	return f
}

// NilChance sets the probability of creating a nil pointer, map, or slice to
// 'p'. 'p' should be between 0 (no nils) and 1 (all nils), inclusive.
func (f *GreenRunner) NilChance(p float64) *GreenRunner {
	if p < 0 || p > 1 {
		panic("p should be between 0 and 1, inclusive.")
	}
	f.nilChance = p
	return f
}

// NumElements sets the minimum and maximum number of elements that will be
// added to a non-nil map or slice.
func (f *GreenRunner) NumElements(atLeast, atMost int) *GreenRunner {
	if atLeast > atMost {
		panic("atLeast must be <= atMost")
	}
	if atLeast < 0 {
		panic("atLeast must be >= 0")
	}
	f.minElements = atLeast
	f.maxElements = atMost
	return f
}

func (f *GreenRunner) genElementCount() int {
	if f.minElements == f.maxElements {
		return f.minElements
	}
	return f.minElements + f.r.Intn(f.maxElements-f.minElements+1)
}

func (f *GreenRunner) genShouldFill() bool {
	return f.r.Float64() > f.nilChance
}

// MaxDepth sets the maximum number of recursive greenrun calls that will be made
// before stopping.  This includes struct members, pointers, and map and slice
// elements.
func (f *GreenRunner) MaxDepth(d int) *GreenRunner {
	f.maxDepth = d
	return f
}

// GreenRun recursively fills all of obj's fields with something random.  First
// this tries to find a custom greenrun function (see Funcs).  If there is no
// custom function this tests whether the object implements greenrun.Interface and,
// if so, calls GreenRun on it to greenrun itself.  If that fails, this will see if
// there is a default greenrun function provided by this package.  If all of that
// fails, this will generate random values for all primitive fields and then
// recurse for all non-primitives.
//
// This is safe for cyclic or tree-like structs, up to a limit.  Use the
// MaxDepth method to adjust how deep you need it to recurse.
//
// obj must be a pointer. Only exported (public) fields can be set (thanks,
// golang :/ ) Intended for tests, so will panic on bad input or unimplemented
// fields.
func (f *GreenRunner) GreenRun(obj interface{}) {
	v := reflect.ValueOf(obj)
	if v.Kind() != reflect.Ptr {
		panic("needed ptr!")
	}
	v = v.Elem()
	f.greenrunWithContext(v, 0)
}

// GreenRunNoCustom is just like GreenRun, except that any custom greenrun function for
// obj's type will not be called and obj will not be tested for greenrun.Interface
// conformance.  This applies only to obj and not other instances of obj's
// type.
// Not safe for cyclic or tree-like structs!
// obj must be a pointer. Only exported (public) fields can be set (thanks, golang :/ )
// Intended for tests, so will panic on bad input or unimplemented fields.
func (f *GreenRunner) GreenRunNoCustom(obj interface{}) {
	v := reflect.ValueOf(obj)
	if v.Kind() != reflect.Ptr {
		panic("needed ptr!")
	}
	v = v.Elem()
	f.greenrunWithContext(v, flagNoCustomGreenRun)
}

const (
	// Do not try to find a custom greenrun function.  Does not apply recursively.
	flagNoCustomGreenRun uint64 = 1 << iota
)

func (f *GreenRunner) greenrunWithContext(v reflect.Value, flags uint64) {
	fc := &greenrunerContext{greenruner: f}
	fc.doGreenRun(v, flags)
}

// greenrunerContext carries context about a single greenruning run, which lets GreenRunner
// be thread-safe.
type greenrunerContext struct {
	greenruner   *GreenRunner
	curDepth int
}

func (fc *greenrunerContext) doGreenRun(v reflect.Value, flags uint64) {
	if fc.curDepth >= fc.greenruner.maxDepth {
		return
	}
	fc.curDepth++
	defer func() { fc.curDepth-- }()

	if !v.CanSet() {
		return
	}

	if flags&flagNoCustomGreenRun == 0 {
		// Check for both pointer and non-pointer custom functions.
		if v.CanAddr() && fc.tryCustom(v.Addr()) {
			return
		}
		if fc.tryCustom(v) {
			return
		}
	}

	if fn, ok := fillFuncMap[v.Kind()]; ok {
		fn(v, fc.greenruner.r)
		return
	}
	switch v.Kind() {
	case reflect.Map:
		if fc.greenruner.genShouldFill() {
			v.Set(reflect.MakeMap(v.Type()))
			n := fc.greenruner.genElementCount()
			for i := 0; i < n; i++ {
				key := reflect.New(v.Type().Key()).Elem()
				fc.doGreenRun(key, 0)
				val := reflect.New(v.Type().Elem()).Elem()
				fc.doGreenRun(val, 0)
				v.SetMapIndex(key, val)
			}
			return
		}
		v.Set(reflect.Zero(v.Type()))
	case reflect.Ptr:
		if fc.greenruner.genShouldFill() {
			v.Set(reflect.New(v.Type().Elem()))
			fc.doGreenRun(v.Elem(), 0)
			return
		}
		v.Set(reflect.Zero(v.Type()))
	case reflect.Slice:
		if fc.greenruner.genShouldFill() {
			n := fc.greenruner.genElementCount()
			v.Set(reflect.MakeSlice(v.Type(), n, n))
			for i := 0; i < n; i++ {
				fc.doGreenRun(v.Index(i), 0)
			}
			return
		}
		v.Set(reflect.Zero(v.Type()))
	case reflect.Array:
		if fc.greenruner.genShouldFill() {
			n := v.Len()
			for i := 0; i < n; i++ {
				fc.doGreenRun(v.Index(i), 0)
			}
			return
		}
		v.Set(reflect.Zero(v.Type()))
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			fc.doGreenRun(v.Field(i), 0)
		}
	case reflect.Chan:
		fallthrough
	case reflect.Func:
		fallthrough
	case reflect.Interface:
		fallthrough
	default:
		panic(fmt.Sprintf("Can't handle %#v", v.Interface()))
	}
}

// tryCustom searches for custom handlers, and returns true iff it finds a match
// and successfully randomizes v.
func (fc *greenrunerContext) tryCustom(v reflect.Value) bool {
	// First: see if we have a greenrun function for it.
	doCustom, ok := fc.greenruner.greenrunFuncs[v.Type()]
	if !ok {
		// Second: see if it can greenrun itself.
		if v.CanInterface() {
			intf := v.Interface()
			if greenrunable, ok := intf.(Interface); ok {
				greenrunable.GreenRun(Continue{fc: fc, Rand: fc.greenruner.r})
				return true
			}
		}
		// Finally: see if there is a default greenrun function.
		doCustom, ok = fc.greenruner.defaultGreenRunFuncs[v.Type()]
		if !ok {
			return false
		}
	}

	switch v.Kind() {
	case reflect.Ptr:
		if v.IsNil() {
			if !v.CanSet() {
				return false
			}
			v.Set(reflect.New(v.Type().Elem()))
		}
	case reflect.Map:
		if v.IsNil() {
			if !v.CanSet() {
				return false
			}
			v.Set(reflect.MakeMap(v.Type()))
		}
	default:
		return false
	}

	doCustom.Call([]reflect.Value{v, reflect.ValueOf(Continue{
		fc:   fc,
		Rand: fc.greenruner.r,
	})})
	return true
}

// Interface represents an object that knows how to greenrun itself.  Any time we
// find a type that implements this interface we will delegate the act of
// greenruning itself.
type Interface interface {
	GreenRun(c Continue)
}

// Continue can be passed to custom greenruning functions to allow them to use
// the correct source of randomness and to continue greenruning their members.
type Continue struct {
	fc *greenrunerContext

	// For convenience, Continue implements rand.Rand via embedding.
	// Use this for generating any randomness if you want your greenruning
	// to be repeatable for a given seed.
	*rand.Rand
}

// GreenRun continues greenruning obj. obj must be a pointer.
func (c Continue) GreenRun(obj interface{}) {
	v := reflect.ValueOf(obj)
	if v.Kind() != reflect.Ptr {
		panic("needed ptr!")
	}
	v = v.Elem()
	c.fc.doGreenRun(v, 0)
}

// GreenRunNoCustom continues greenruning obj, except that any custom greenrun function for
// obj's type will not be called and obj will not be tested for greenrun.Interface
// conformance.  This applies only to obj and not other instances of obj's
// type.
func (c Continue) GreenRunNoCustom(obj interface{}) {
	v := reflect.ValueOf(obj)
	if v.Kind() != reflect.Ptr {
		panic("needed ptr!")
	}
	v = v.Elem()
	c.fc.doGreenRun(v, flagNoCustomGreenRun)
}

// RandString makes a random string up to 20 characters long. The returned string
// may include a variety of (valid) UTF-8 encodings.
func (c Continue) RandString() string {
	return randString(c.Rand)
}

// RandUint64 makes random 64 bit numbers.
// Weirdly, rand doesn't have a function that gives you 64 random bits.
func (c Continue) RandUint64() uint64 {
	return randUint64(c.Rand)
}

// RandBool returns true or false randomly.
func (c Continue) RandBool() bool {
	return randBool(c.Rand)
}

func greenrunInt(v reflect.Value, r *rand.Rand) {
	v.SetInt(int64(randUint64(r)))
}

func greenrunUint(v reflect.Value, r *rand.Rand) {
	v.SetUint(randUint64(r))
}

func greenrunTime(t *time.Time, c Continue) {
	var sec, nsec int64
	// Allow for about 1000 years of random time values, which keeps things
	// like JSON parsing reasonably happy.
	sec = c.Rand.Int63n(1000 * 365 * 24 * 60 * 60)
	c.GreenRun(&nsec)
	*t = time.Unix(sec, nsec)
}

var fillFuncMap = map[reflect.Kind]func(reflect.Value, *rand.Rand){
	reflect.Bool: func(v reflect.Value, r *rand.Rand) {
		v.SetBool(randBool(r))
	},
	reflect.Int:     greenrunInt,
	reflect.Int8:    greenrunInt,
	reflect.Int16:   greenrunInt,
	reflect.Int32:   greenrunInt,
	reflect.Int64:   greenrunInt,
	reflect.Uint:    greenrunUint,
	reflect.Uint8:   greenrunUint,
	reflect.Uint16:  greenrunUint,
	reflect.Uint32:  greenrunUint,
	reflect.Uint64:  greenrunUint,
	reflect.Uintptr: greenrunUint,
	reflect.Float32: func(v reflect.Value, r *rand.Rand) {
		v.SetFloat(float64(r.Float32()))
	},
	reflect.Float64: func(v reflect.Value, r *rand.Rand) {
		v.SetFloat(r.Float64())
	},
	reflect.Complex64: func(v reflect.Value, r *rand.Rand) {
		panic("unimplemented")
	},
	reflect.Complex128: func(v reflect.Value, r *rand.Rand) {
		panic("unimplemented")
	},
	reflect.String: func(v reflect.Value, r *rand.Rand) {
		v.SetString(randString(r))
	},
	reflect.UnsafePointer: func(v reflect.Value, r *rand.Rand) {
		panic("unimplemented")
	},
}

// randBool returns true or false randomly.
func randBool(r *rand.Rand) bool {
	if r.Int()&1 == 1 {
		return true
	}
	return false
}

type charRange struct {
	first, last rune
}

// choose returns a random unicode character from the given range, using the
// given randomness source.
func (r *charRange) choose(rand *rand.Rand) rune {
	count := int64(r.last - r.first)
	return r.first + rune(rand.Int63n(count))
}

var unicodeRanges = []charRange{
	{' ', '~'},           // ASCII characters
	{'\u00a0', '\u02af'}, // Multi-byte encoded characters
	{'\u4e00', '\u9fff'}, // Common CJK (even longer encodings)
}

// randString makes a random string up to 20 characters long. The returned string
// may include a variety of (valid) UTF-8 encodings.
func randString(r *rand.Rand) string {
	n := r.Intn(20)
	runes := make([]rune, n)
	for i := range runes {
		runes[i] = unicodeRanges[r.Intn(len(unicodeRanges))].choose(r)
	}
	return string(runes)
}

// randUint64 makes random 64 bit numbers.
// Weirdly, rand doesn't have a function that gives you 64 random bits.
func randUint64(r *rand.Rand) uint64 {
	return uint64(r.Uint32())<<32 | uint64(r.Uint32())
}
