package mineru

import (
	"context"

	"google.golang.org/grpc"
)

// This file contains a "stunt" implementation of the gRPC generated code
// to allow the project to compile without running protoc.

type DocumentIntelligenceClient interface {
	ParseDocument(ctx context.Context, in *ParseRequest, opts ...grpc.CallOption) (*ParseResponse, error)
	Heartbeat(ctx context.Context, in *Empty, opts ...grpc.CallOption) (*StatusResponse, error)
}

type documentIntelligenceClient struct {
	cc grpc.ClientConnInterface
}

func NewDocumentIntelligenceClient(cc grpc.ClientConnInterface) DocumentIntelligenceClient {
	return &documentIntelligenceClient{cc}
}

func (c *documentIntelligenceClient) ParseDocument(ctx context.Context, in *ParseRequest, opts ...grpc.CallOption) (*ParseResponse, error) {
	out := new(ParseResponse)
	err := c.cc.Invoke(ctx, "/mineru.DocumentIntelligence/ParseDocument", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *documentIntelligenceClient) Heartbeat(ctx context.Context, in *Empty, opts ...grpc.CallOption) (*StatusResponse, error) {
	out := new(StatusResponse)
	err := c.cc.Invoke(ctx, "/mineru.DocumentIntelligence/Heartbeat", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

type ParseRequest struct {
	FileContent   []byte `protobuf:"bytes,1,opt,name=file_content,json=fileContent,proto3" json:"file_content,omitempty"`
	FileName      string `protobuf:"bytes,2,opt,name=file_name,json=fileName,proto3" json:"file_name,omitempty"`
	ForceOcr      bool   `protobuf:"varint,3,opt,name=force_ocr,json=forceOcr,proto3" json:"force_ocr,omitempty"`
	DisableLayout bool   `protobuf:"varint,4,opt,name=disable_layout,json=disableLayout,proto3" json:"disable_layout,omitempty"`
}

type ParseResponse struct {
	Markdown    string  `protobuf:"bytes,1,opt,name=markdown,proto3" json:"markdown,omitempty"`
	LayoutJson  string  `protobuf:"bytes,2,opt,name=layout_json,json=layoutJson,proto3" json:"layout_json,omitempty"`
	ErrorMsg    string  `protobuf:"bytes,3,opt,name=error_msg,json=errorMsg,proto3" json:"error_msg,omitempty"`
	ProcessTime float64 `protobuf:"fixed64,4,opt,name=process_time,json=processTime,proto3" json:"process_time,omitempty"`
}

type Empty struct{}

type StatusResponse struct {
	Healthy        bool   `protobuf:"varint,1,opt,name=healthy,proto3" json:"healthy,omitempty"`
	Status         string `protobuf:"bytes,2,opt,name=status,proto3" json:"status,omitempty"`
	ConcurrentJobs int32  `protobuf:"varint,3,opt,name=concurrent_jobs,json=concurrentJobs,proto3" json:"concurrent_jobs,omitempty"`
}

// Dummy methods to satisfy protoreflect etc if needed (simplified)
func (*ParseRequest) Reset()         {}
func (*ParseRequest) String() string { return "" }
func (*ParseRequest) ProtoMessage()  {}

func (*ParseResponse) Reset()         {}
func (*ParseResponse) String() string { return "" }
func (*ParseResponse) ProtoMessage()  {}

func (*Empty) Reset()         {}
func (*Empty) String() string { return "" }
func (*Empty) ProtoMessage()  {}

func (*StatusResponse) Reset()         {}
func (*StatusResponse) String() string { return "" }
func (*StatusResponse) ProtoMessage()  {}
