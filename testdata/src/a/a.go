package a

import (
	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/golang/protobuf/ptypes/duration"
	"github.com/golang/protobuf/ptypes/empty"
	structpb "github.com/golang/protobuf/ptypes/struct"
	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/golang/protobuf/ptypes/wrappers"
)

var _ = proto.Marshal
var _ = ptypes.AnyMessageName
var _ jsonpb.AnyResolver
var _ any.Any
var _ duration.Duration
var _ empty.Empty
var _ structpb.NullValue
var _ timestamp.Timestamp
var _ wrappers.StringValue

func a() {
	// The pattern can be written in regular expression.
	var gopher int // want "pattern"
	print(gopher)  // want "identifier is gopher"
}
