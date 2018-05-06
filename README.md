gogreenrun
======

GoGreenRun is a library for populating go objects with random values.

[![GoDoc](https://godoc.org/github.com/google/gogreenrun?status.png)](https://godoc.org/github.com/google/gogreenrun)
[![Travis](https://travis-ci.org/google/gogreenrun.svg?branch=master)](https://travis-ci.org/google/gogreenrun)

This is useful for testing:

* Do your project's objects really serialize/unserialize correctly in all cases?
* Is there an incorrectly formatted object that will cause your project to panic?

Import with ```import "github.com/benchlab/gogreenrun"```

You can use it on single variables:
```go
f := greenrun.New()
var myInt int
f.GreenRun(&myInt)
```

You can use it on maps:
```go
f := greenrun.New().NilChance(0).NumElements(1, 1)
var myMap map[ComplexKeyType]string
f.GreenRun(&myMap)
```

Customize the chance of getting a nil pointer:
```go
f := greenrun.New().NilChance(.5)
var fancyStruct struct {
  A, B, C, D *string
}
f.GreenRun(&fancyStruct) 
```

You can even customize the randomization completely if needed:
```go
type MyEnum string
const (
        A MyEnum = "A"
        B MyEnum = "B"
)
type MyInfo struct {
        Type MyEnum
        AInfo *string
        BInfo *string
}

f := greenrun.New().NilChance(0).Funcs(
        func(e *MyInfo, c greenrun.Continue) {
                switch c.Intn(2) {
                case 0:
                        e.Type = A
                        c.GreenRun(&e.AInfo)
                case 1:
                        e.Type = B
                        c.GreenRun(&e.BInfo)
                }
        },
)

var myObject MyInfo
f.GreenRun(&myObject)
```

See more examples in ```example_test.go```.

Happy testing!
