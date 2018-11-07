package plugin

import (
	"gopkg.in/sensorbee/pymlstate.v0"
	"gopkg.in/sensorbee/sensorbee.v0/bql/udf"
)

func init() {
	udf.MustRegisterGlobalUDSCreator("pymlstate", &pymlstate.StateCreator{})

	udf.MustRegisterGlobalUDF("pymlstate_fit",
		udf.MustConvertGeneric(pymlstate.Fit))
	udf.MustRegisterGlobalUDF("pymlstate_predict",
		udf.MustConvertGeneric(pymlstate.Predict))
	udf.MustRegisterGlobalUDF("pymlstate_flush",
		udf.MustConvertGeneric(pymlstate.Flush))
	udf.MustRegisterGlobalUDF("pymlstate_call",
		udf.MustConvertGeneric(pymlstate.CallMethod))
}
