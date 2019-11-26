module github.com/wI2L/jettison

go 1.12

replace git.apache.org/thrift.git => github.com/apache/thrift v0.12.0

replace sourcegraph.com/sourcegraph/go-diff v0.5.1 => github.com/sourcegraph/go-diff v0.5.1

replace github.com/golang/lint => golang.org/x/lint v0.0.0-20190909230951-414d861bb4ac

require (
	github.com/francoispqt/gojay v1.2.13
	github.com/json-iterator/go v1.1.7
	github.com/modern-go/reflect2 v1.0.1
	github.com/stretchr/testify v1.4.0 // indirect
	gopkg.in/check.v1 v1.0.0-20190902080502-41f04d3bba15 // indirect
	gopkg.in/yaml.v2 v2.2.4 // indirect
)
