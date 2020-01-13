package jettison

import (
	"strconv"
	"unsafe"
)

// nolint:unparam
func encodeInt(
	p unsafe.Pointer, dst []byte, _ encOpts,
) ([]byte, error) {
	return strconv.AppendInt(dst, int64(*(*int)(p)), 10), nil
}

// nolint:unparam
func encodeInt8(
	p unsafe.Pointer, dst []byte, _ encOpts,
) ([]byte, error) {
	return strconv.AppendInt(dst, int64(*(*int8)(p)), 10), nil
}

// nolint:unparam
func encodeInt16(
	p unsafe.Pointer, dst []byte, _ encOpts,
) ([]byte, error) {
	return strconv.AppendInt(dst, int64(*(*int16)(p)), 10), nil
}

// nolint:unparam
func encodeInt32(
	p unsafe.Pointer, dst []byte, _ encOpts,
) ([]byte, error) {
	return strconv.AppendInt(dst, int64(*(*int32)(p)), 10), nil
}

// nolint:unparam
func encodeInt64(
	p unsafe.Pointer, dst []byte, _ encOpts,
) ([]byte, error) {
	return strconv.AppendInt(dst, *(*int64)(p), 10), nil
}

// nolint:unparam
func encodeUint(
	p unsafe.Pointer, dst []byte, _ encOpts,
) ([]byte, error) {
	return strconv.AppendUint(dst, uint64(*(*uint)(p)), 10), nil
}

// nolint:unparam
func encodeUint8(
	p unsafe.Pointer, dst []byte, _ encOpts,
) ([]byte, error) {
	return strconv.AppendUint(dst, uint64(*(*uint8)(p)), 10), nil
}

// nolint:unparam
func encodeUint16(
	p unsafe.Pointer, dst []byte, _ encOpts,
) ([]byte, error) {
	return strconv.AppendUint(dst, uint64(*(*uint16)(p)), 10), nil
}

// nolint:unparam
func encodeUint32(
	p unsafe.Pointer, dst []byte, _ encOpts,
) ([]byte, error) {
	return strconv.AppendUint(dst, uint64(*(*uint32)(p)), 10), nil
}

// nolint:unparam
func encodeUint64(
	p unsafe.Pointer, dst []byte, _ encOpts,
) ([]byte, error) {
	return strconv.AppendUint(dst, *(*uint64)(p), 10), nil
}

// nolint:unparam
func encodeUintptr(
	p unsafe.Pointer, dst []byte, _ encOpts,
) ([]byte, error) {
	return strconv.AppendUint(dst, uint64(*(*uintptr)(p)), 10), nil
}
