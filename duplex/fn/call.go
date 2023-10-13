package fn

import (
	"fmt"
	"reflect"

	"github.com/mitchellh/mapstructure"
)

var errorInterface = reflect.TypeOf((*error)(nil)).Elem()

// Call wraps invoking a function via reflection, converting the arguments with
// ArgsTo and the returns with ParseReturn. fn argument can be a function
// or a reflect.Value for a function.
func Call(fn any, args []any) (_ []any, err error) {
	fnval := reflect.ValueOf(fn)
	if rv, ok := fn.(reflect.Value); ok {
		fnval = rv
	}
	fnParams, err := ArgsTo(fnval.Type(), args)
	if err != nil {
		return nil, err
	}
	fnReturn := fnval.Call(fnParams)
	return ParseReturn(fnReturn)
}

// ArgsTo converts the arguments into `reflect.Value`s suitable to pass as
// parameters to a function with the given type via reflection.
func ArgsTo(fntyp reflect.Type, args []any) ([]reflect.Value, error) {
	if len(args) != fntyp.NumIn() {
		return nil, fmt.Errorf("fn: expected %d params, got %d", fntyp.NumIn(), len(args))
	}
	fnParams := make([]reflect.Value, len(args))
	for idx, param := range args {
		switch fntyp.In(idx).Kind() {
		case reflect.Struct:
			// decode to struct type using mapstructure
			arg := reflect.New(fntyp.In(idx))
			if err := mapstructure.Decode(param, arg.Interface()); err != nil {
				return nil, fmt.Errorf("fn: mapstructure: %s", err.Error())
			}
			fnParams[idx] = ensureType(arg.Elem(), fntyp.In(idx))
		case reflect.Slice:
			rv := reflect.ValueOf(param)
			// decode slice of structs to struct type using mapstructure
			if fntyp.In(idx).Elem().Kind() == reflect.Struct {
				nv := reflect.MakeSlice(fntyp.In(idx), rv.Len(), rv.Len())
				for i := 0; i < rv.Len(); i++ {
					ref := reflect.New(nv.Index(i).Type())
					if err := mapstructure.Decode(rv.Index(i).Interface(), ref.Interface()); err != nil {
						return nil, fmt.Errorf("fn: mapstructure: %s", err.Error())
					}
					nv.Index(i).Set(reflect.Indirect(ref))
				}
				rv = nv
			}
			fnParams[idx] = rv
		default:
			// if int is expected but got float64 assume json-like encoding and cast float to int
			if fntyp.In(idx).Kind() == reflect.Int && reflect.TypeOf(param).Kind() == reflect.Float64 {
				param = int(param.(float64))
			}
			fnParams[idx] = ensureType(reflect.ValueOf(param), fntyp.In(idx))
		}
	}
	return fnParams, nil
}

// ParseReturn splits the results of reflect.Call() into the values, and
// possibly an error.
// If the last value is a non-nil error, this will return `nil, err`.
// If the last value is a nil error it will be removed from the value list.
// Any remaining values will be converted and returned as `any` typed values.
func ParseReturn(ret []reflect.Value) ([]any, error) {
	if len(ret) == 0 {
		return nil, nil
	}
	last := ret[len(ret)-1]
	if last.Type().Implements(errorInterface) {
		if !last.IsNil() {
			return nil, last.Interface().(error)
		}
		ret = ret[:len(ret)-1]
	}
	out := make([]any, len(ret))
	for i, r := range ret {
		out[i] = r.Interface()
	}
	return out, nil
}

// ensureType ensures a value is converted to the expected
// defined type from a convertable underlying type
func ensureType(v reflect.Value, t reflect.Type) reflect.Value {
	if !v.IsValid() {
		// handle nil values with zero value of expected type
		return reflect.New(t).Elem()
	}
	nv := v
	if v.Type().Kind() == reflect.Slice && v.Type().Elem() != t {
		switch t.Kind() {
		case reflect.Array:
			nv = reflect.Indirect(reflect.New(t))
			for i := 0; i < v.Len(); i++ {
				vv := reflect.ValueOf(v.Index(i).Interface())
				nv.Index(i).Set(vv.Convert(nv.Type().Elem()))
			}
		case reflect.Slice:
			nv = reflect.MakeSlice(t, 0, 0)
			for i := 0; i < v.Len(); i++ {
				vv := reflect.ValueOf(v.Index(i).Interface())
				nv = reflect.Append(nv, vv.Convert(nv.Type().Elem()))
			}
		default:
			panic("unable to convert slice to non-array, non-slice type")
		}
	}
	if v.Type() != t {
		nv = nv.Convert(t)
	}
	return nv
}
