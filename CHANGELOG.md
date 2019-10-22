# Changelog

All notable changes to this project are documented in this file.

**THIS LIBRARY IS STILL IN ALPHA AND THERE ARE NO GUARANTEES REGARDING API STABILITY YET**

## [v0.4.1] - 2019-10-23
- Fix unsafe misuses reported by go vet and the new `-d=checkptr` cmd/compile flag introduced in the Go1.14 development tree by Matthew Dempsky. The issues were mostly related to invalid arithmetic operations and dereferences.
- Fix map key types precedence order during marshaling. Keys of any string type are used directly instead of the `MarshalText` method, if the types also implement the `encoding.TextMarshaler` interface.

## [v0.4.0] - 2019-10-18
- Add the `Marshaler` interface. Types that implements it can write a JSON representation of themselves to a `Writer` directly, to avoid having to allocate a buffer as they would usually do when using the `json.Marshaler` interface.

## [v0.3.1] - 2019-10-09
- Fix HTML characters escaping in struct field names.
- Add examples for Marshal, MarshalTo and Encoder's Encode.
- Refactor string encoding to be compliant with `encoding/json`.

## [v0.3.0] - 2019-09-23
- Add global functions `Marshal`, `MarshalTo` and `Register`.
- Update `README.md`: usage, examples and benchmarks.

## [v0.2.1] - 2019-09-10
- Refactor instructions for types implementing the `json.Marshaler` and `encoding.TextMarshaler` interfaces.
   - Fix encoding of `nil` instances.
   - Fix behavior for pointer and non-pointer receivers, to comply with `encoding/json`.
- Fix bug that prevents tagged fields to dominate untagged fields.
- Add support for anonymous struct pointer fields.
- Improve tests coverage of `encoder.go`.
- Add test cases for unexported non-embedded struct fields.

## [v0.2.0] - 2019-09-01
- Add support for `json.Number`.
- Update `README.md` to add a Go1.12+ requirement notice.

## [v0.1.0] - 2019-08-30
Initial realease.

[v0.4.1]: https://github.com/wI2L/jettison/compare/v0.4.0...v0.4.1
[v0.4.0]: https://github.com/wI2L/jettison/compare/v0.3.1...v0.4.0
[v0.3.1]: https://github.com/wI2L/jettison/compare/v0.3.0...v0.3.1
[v0.3.0]: https://github.com/wI2L/jettison/compare/v0.2.1...v0.3.0
[v0.2.1]: https://github.com/wI2L/jettison/compare/v0.2.0...v0.2.1
[v0.2.0]: https://github.com/wI2L/jettison/compare/0.1.0...v0.2.0
[v0.1.0]: https://github.com/wI2L/jettison/releases/tag/0.1.0
