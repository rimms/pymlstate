package mlstate

import (
	"fmt"
	"pfi/sensorbee/pystate/py"
	"pfi/sensorbee/sensorbee/core"
	"pfi/sensorbee/sensorbee/data"
)

var (
	lossPath = data.MustCompilePath("loss")
	accPath  = data.MustCompilePath("accuracy")
)

// PyMLState is python instance specialized to multiple layer classification.
type PyMLState struct {
	mdl py.ObjectModule
	ins py.ObjectInstance

	bucket    data.Array
	batchSize int
}

// NewPyMLState creates `core.SharedState` for multiple layer classification.
func NewPyMLState(modulePathName, moduleName, className string, batchSize int,
	modelPath string) (*PyMLState, error) {
	py.ImportSysAndAppendPath(modulePathName)

	mdl, err := py.LoadModule(moduleName)
	if err != nil {
		return nil, err
	}

	ins, err := mdl.NewInstance(className, data.String(modelPath))
	if err != nil {
		mdl.DecRef()
		return nil, err
	}

	return &PyMLState{
		mdl:       mdl,
		ins:       ins,
		bucket:    make(data.Array, 0, batchSize),
		batchSize: batchSize,
	}, nil
}

// Terminate this state.
func (s *PyMLState) Terminate(ctx *core.Context) error {
	s.ins.DecRef()
	s.mdl.DecRef()
	return nil
}

// Write and call "fit" function. Tuples is cached per train batch size.
func (s *PyMLState) Write(ctx *core.Context, t *core.Tuple) error {
	s.bucket = append(s.bucket, t.Data)

	var err error
	if len(s.bucket) >= s.batchSize {
		m, er := s.Fit(ctx, s.bucket)
		err = er
		s.bucket = s.bucket[:0] // clear slice but keep capacity

		// optional logging, return non-error even if the value does not have
		// accuracy and loss.
		if ret, er := data.AsMap(m); er == nil {
			var loss float64
			if l, e := ret.Get(lossPath); e != nil {
				return err
			} else if loss, e = data.ToFloat(l); e != nil {
				return err
			}
			var acc float64
			if a, e := ret.Get(accPath); e != nil {
				return err
			} else if acc, e = data.ToFloat(a); e != nil {
				return err
			}
			ctx.Log().Debugf("loss=%.3f acc=%.3f", loss/float64(s.batchSize),
				acc/float64(s.batchSize))
		}
	}

	return err
}

// Fit receives `data.Array` type but it assumes `[]data.Map` type
// for passing arguments to `fit` method.
func (s *PyMLState) Fit(ctx *core.Context, bucket data.Array) (data.Value, error) {
	return s.ins.Call("fit", bucket)
}

// FitMap receives `[]data.Map`, these maps are converted to `data.Array`
func (s *PyMLState) FitMap(ctx *core.Context, bucket []data.Map) (data.Value, error) {
	args := make(data.Array, len(bucket))
	for i, v := range bucket {
		args[i] = v
	}
	return s.ins.Call("fit", args)
}

// PyMLFit fits buckets. fit algorithm and return value is depends on Python
// implementation.
func PyMLFit(ctx *core.Context, stateName string, bucket []data.Map) (data.Value, error) {
	s, err := lookupPyMLState(ctx, stateName)
	if err != nil {
		return nil, err
	}

	return s.FitMap(ctx, bucket)
}

// PyMLPredict predicts data and return estimate value.
func PyMLPredict(ctx *core.Context, stateName string, dt data.Value) (data.Value, error) {
	s, err := lookupPyMLState(ctx, stateName)
	if err != nil {
		return nil, err
	}

	return s.ins.Call("predict", dt)
}

func lookupPyMLState(ctx *core.Context, stateName string) (*PyMLState, error) {
	st, err := ctx.SharedStates.Get(stateName)
	if err != nil {
		return nil, err
	}

	if s, ok := st.(*PyMLState); ok {
		return s, nil
	}

	return nil, fmt.Errorf("state '%v' isn't a PyMLState", stateName)
}
