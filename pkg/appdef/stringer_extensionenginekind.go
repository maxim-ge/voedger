// Code generated by "stringer -type=ExtensionEngineKind -output=stringer_extensionenginekind.go"; DO NOT EDIT.

package appdef

import "strconv"

func _() {
	// An "invalid array index" compiler error signifies that the constant values have changed.
	// Re-run the stringer command to generate them again.
	var x [1]struct{}
	_ = x[ExtensionEngineKind_null-0]
	_ = x[ExtensionEngineKind_BuiltIn-1]
	_ = x[ExtensionEngineKind_WASM-2]
	_ = x[ExtensionEngineKind_count-3]
}

const _ExtensionEngineKind_name = "ExtensionEngineKind_nullExtensionEngineKind_BuiltInExtensionEngineKind_WASMExtensionEngineKind_count"

var _ExtensionEngineKind_index = [...]uint8{0, 24, 51, 75, 100}

func (i ExtensionEngineKind) String() string {
	if i >= ExtensionEngineKind(len(_ExtensionEngineKind_index)-1) {
		return "ExtensionEngineKind(" + strconv.FormatInt(int64(i), 10) + ")"
	}
	return _ExtensionEngineKind_name[_ExtensionEngineKind_index[i]:_ExtensionEngineKind_index[i+1]]
}
