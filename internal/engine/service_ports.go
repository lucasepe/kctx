package engine

import (
	"strconv"

	"k8s.io/apimachinery/pkg/util/intstr"
)

// targetPortString preserves a Service targetPort whether it was declared as a
// number or a named port.
func targetPortString(port intstr.IntOrString) string {
	switch port.Type {
	case intstr.Int:
		if port.IntVal == 0 {
			return ""
		}
		return strconv.FormatInt(int64(port.IntVal), 10)
	case intstr.String:
		return port.StrVal
	default:
		return ""
	}
}
