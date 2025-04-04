// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.36.5
// 	protoc        (unknown)
// source: proto/v1/vsapi.proto

package v1

import (
	_ "google.golang.org/genproto/googleapis/api/annotations"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
	reflect "reflect"
	sync "sync"
	unsafe "unsafe"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type PlayerStatsRequest struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Name          string                 `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *PlayerStatsRequest) Reset() {
	*x = PlayerStatsRequest{}
	mi := &file_proto_v1_vsapi_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *PlayerStatsRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*PlayerStatsRequest) ProtoMessage() {}

func (x *PlayerStatsRequest) ProtoReflect() protoreflect.Message {
	mi := &file_proto_v1_vsapi_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use PlayerStatsRequest.ProtoReflect.Descriptor instead.
func (*PlayerStatsRequest) Descriptor() ([]byte, []int) {
	return file_proto_v1_vsapi_proto_rawDescGZIP(), []int{0}
}

func (x *PlayerStatsRequest) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

type PlayerStatsResponse struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Name          string                 `protobuf:"bytes,2,opt,name=name,proto3" json:"name,omitempty"`
	DeathCount    int32                  `protobuf:"varint,3,opt,name=death_count,json=deathCount,proto3" json:"death_count,omitempty"`
	HoursPlayed   float32                `protobuf:"fixed32,5,opt,name=hoursPlayed,proto3" json:"hoursPlayed,omitempty"`
	LastOnline    int64                  `protobuf:"varint,6,opt,name=last_online,json=lastOnline,proto3" json:"last_online,omitempty"`
	PlayersKilled int32                  `protobuf:"varint,7,opt,name=players_killed,json=playersKilled,proto3" json:"players_killed,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *PlayerStatsResponse) Reset() {
	*x = PlayerStatsResponse{}
	mi := &file_proto_v1_vsapi_proto_msgTypes[1]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *PlayerStatsResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*PlayerStatsResponse) ProtoMessage() {}

func (x *PlayerStatsResponse) ProtoReflect() protoreflect.Message {
	mi := &file_proto_v1_vsapi_proto_msgTypes[1]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use PlayerStatsResponse.ProtoReflect.Descriptor instead.
func (*PlayerStatsResponse) Descriptor() ([]byte, []int) {
	return file_proto_v1_vsapi_proto_rawDescGZIP(), []int{1}
}

func (x *PlayerStatsResponse) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

func (x *PlayerStatsResponse) GetDeathCount() int32 {
	if x != nil {
		return x.DeathCount
	}
	return 0
}

func (x *PlayerStatsResponse) GetHoursPlayed() float32 {
	if x != nil {
		return x.HoursPlayed
	}
	return 0
}

func (x *PlayerStatsResponse) GetLastOnline() int64 {
	if x != nil {
		return x.LastOnline
	}
	return 0
}

func (x *PlayerStatsResponse) GetPlayersKilled() int32 {
	if x != nil {
		return x.PlayersKilled
	}
	return 0
}

type TimeResponse struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	FormattedTime string                 `protobuf:"bytes,1,opt,name=formatted_time,json=formattedTime,proto3" json:"formatted_time,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *TimeResponse) Reset() {
	*x = TimeResponse{}
	mi := &file_proto_v1_vsapi_proto_msgTypes[2]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *TimeResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*TimeResponse) ProtoMessage() {}

func (x *TimeResponse) ProtoReflect() protoreflect.Message {
	mi := &file_proto_v1_vsapi_proto_msgTypes[2]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use TimeResponse.ProtoReflect.Descriptor instead.
func (*TimeResponse) Descriptor() ([]byte, []int) {
	return file_proto_v1_vsapi_proto_rawDescGZIP(), []int{2}
}

func (x *TimeResponse) GetFormattedTime() string {
	if x != nil {
		return x.FormattedTime
	}
	return ""
}

type PlayersCountResponse struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Count         int32                  `protobuf:"varint,1,opt,name=count,proto3" json:"count,omitempty"`
	Max           int32                  `protobuf:"varint,2,opt,name=max,proto3" json:"max,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *PlayersCountResponse) Reset() {
	*x = PlayersCountResponse{}
	mi := &file_proto_v1_vsapi_proto_msgTypes[3]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *PlayersCountResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*PlayersCountResponse) ProtoMessage() {}

func (x *PlayersCountResponse) ProtoReflect() protoreflect.Message {
	mi := &file_proto_v1_vsapi_proto_msgTypes[3]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use PlayersCountResponse.ProtoReflect.Descriptor instead.
func (*PlayersCountResponse) Descriptor() ([]byte, []int) {
	return file_proto_v1_vsapi_proto_rawDescGZIP(), []int{3}
}

func (x *PlayersCountResponse) GetCount() int32 {
	if x != nil {
		return x.Count
	}
	return 0
}

func (x *PlayersCountResponse) GetMax() int32 {
	if x != nil {
		return x.Max
	}
	return 0
}

type PlayersListResponse struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	PlayerNames   []string               `protobuf:"bytes,1,rep,name=player_names,json=playerNames,proto3" json:"player_names,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *PlayersListResponse) Reset() {
	*x = PlayersListResponse{}
	mi := &file_proto_v1_vsapi_proto_msgTypes[4]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *PlayersListResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*PlayersListResponse) ProtoMessage() {}

func (x *PlayersListResponse) ProtoReflect() protoreflect.Message {
	mi := &file_proto_v1_vsapi_proto_msgTypes[4]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use PlayersListResponse.ProtoReflect.Descriptor instead.
func (*PlayersListResponse) Descriptor() ([]byte, []int) {
	return file_proto_v1_vsapi_proto_rawDescGZIP(), []int{4}
}

func (x *PlayersListResponse) GetPlayerNames() []string {
	if x != nil {
		return x.PlayerNames
	}
	return nil
}

var File_proto_v1_vsapi_proto protoreflect.FileDescriptor

var file_proto_v1_vsapi_proto_rawDesc = string([]byte{
	0x0a, 0x14, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2f, 0x76, 0x31, 0x2f, 0x76, 0x73, 0x61, 0x70, 0x69,
	0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x0a, 0x76, 0x69, 0x6e, 0x74, 0x61, 0x67, 0x65, 0x2e,
	0x76, 0x31, 0x1a, 0x1c, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x61, 0x70, 0x69, 0x2f, 0x61,
	0x6e, 0x6e, 0x6f, 0x74, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f,
	0x1a, 0x17, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x61, 0x70, 0x69, 0x2f, 0x63, 0x6c, 0x69,
	0x65, 0x6e, 0x74, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x1a, 0x1b, 0x67, 0x6f, 0x6f, 0x67, 0x6c,
	0x65, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2f, 0x65, 0x6d, 0x70, 0x74, 0x79,
	0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x22, 0x28, 0x0a, 0x12, 0x50, 0x6c, 0x61, 0x79, 0x65, 0x72,
	0x53, 0x74, 0x61, 0x74, 0x73, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x12, 0x0a, 0x04,
	0x6e, 0x61, 0x6d, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04, 0x6e, 0x61, 0x6d, 0x65,
	0x22, 0xb4, 0x01, 0x0a, 0x13, 0x50, 0x6c, 0x61, 0x79, 0x65, 0x72, 0x53, 0x74, 0x61, 0x74, 0x73,
	0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x12, 0x0a, 0x04, 0x6e, 0x61, 0x6d, 0x65,
	0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x12, 0x1f, 0x0a, 0x0b,
	0x64, 0x65, 0x61, 0x74, 0x68, 0x5f, 0x63, 0x6f, 0x75, 0x6e, 0x74, 0x18, 0x03, 0x20, 0x01, 0x28,
	0x05, 0x52, 0x0a, 0x64, 0x65, 0x61, 0x74, 0x68, 0x43, 0x6f, 0x75, 0x6e, 0x74, 0x12, 0x20, 0x0a,
	0x0b, 0x68, 0x6f, 0x75, 0x72, 0x73, 0x50, 0x6c, 0x61, 0x79, 0x65, 0x64, 0x18, 0x05, 0x20, 0x01,
	0x28, 0x02, 0x52, 0x0b, 0x68, 0x6f, 0x75, 0x72, 0x73, 0x50, 0x6c, 0x61, 0x79, 0x65, 0x64, 0x12,
	0x1f, 0x0a, 0x0b, 0x6c, 0x61, 0x73, 0x74, 0x5f, 0x6f, 0x6e, 0x6c, 0x69, 0x6e, 0x65, 0x18, 0x06,
	0x20, 0x01, 0x28, 0x03, 0x52, 0x0a, 0x6c, 0x61, 0x73, 0x74, 0x4f, 0x6e, 0x6c, 0x69, 0x6e, 0x65,
	0x12, 0x25, 0x0a, 0x0e, 0x70, 0x6c, 0x61, 0x79, 0x65, 0x72, 0x73, 0x5f, 0x6b, 0x69, 0x6c, 0x6c,
	0x65, 0x64, 0x18, 0x07, 0x20, 0x01, 0x28, 0x05, 0x52, 0x0d, 0x70, 0x6c, 0x61, 0x79, 0x65, 0x72,
	0x73, 0x4b, 0x69, 0x6c, 0x6c, 0x65, 0x64, 0x22, 0x35, 0x0a, 0x0c, 0x54, 0x69, 0x6d, 0x65, 0x52,
	0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x25, 0x0a, 0x0e, 0x66, 0x6f, 0x72, 0x6d, 0x61,
	0x74, 0x74, 0x65, 0x64, 0x5f, 0x74, 0x69, 0x6d, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52,
	0x0d, 0x66, 0x6f, 0x72, 0x6d, 0x61, 0x74, 0x74, 0x65, 0x64, 0x54, 0x69, 0x6d, 0x65, 0x22, 0x3e,
	0x0a, 0x14, 0x50, 0x6c, 0x61, 0x79, 0x65, 0x72, 0x73, 0x43, 0x6f, 0x75, 0x6e, 0x74, 0x52, 0x65,
	0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x14, 0x0a, 0x05, 0x63, 0x6f, 0x75, 0x6e, 0x74, 0x18,
	0x01, 0x20, 0x01, 0x28, 0x05, 0x52, 0x05, 0x63, 0x6f, 0x75, 0x6e, 0x74, 0x12, 0x10, 0x0a, 0x03,
	0x6d, 0x61, 0x78, 0x18, 0x02, 0x20, 0x01, 0x28, 0x05, 0x52, 0x03, 0x6d, 0x61, 0x78, 0x22, 0x38,
	0x0a, 0x13, 0x50, 0x6c, 0x61, 0x79, 0x65, 0x72, 0x73, 0x4c, 0x69, 0x73, 0x74, 0x52, 0x65, 0x73,
	0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x21, 0x0a, 0x0c, 0x70, 0x6c, 0x61, 0x79, 0x65, 0x72, 0x5f,
	0x6e, 0x61, 0x6d, 0x65, 0x73, 0x18, 0x01, 0x20, 0x03, 0x28, 0x09, 0x52, 0x0b, 0x70, 0x6c, 0x61,
	0x79, 0x65, 0x72, 0x4e, 0x61, 0x6d, 0x65, 0x73, 0x32, 0x83, 0x06, 0x0a, 0x0e, 0x56, 0x69, 0x6e,
	0x74, 0x61, 0x67, 0x65, 0x53, 0x65, 0x72, 0x76, 0x69, 0x63, 0x65, 0x12, 0x51, 0x0a, 0x0b, 0x47,
	0x65, 0x74, 0x47, 0x61, 0x6d, 0x65, 0x54, 0x69, 0x6d, 0x65, 0x12, 0x16, 0x2e, 0x67, 0x6f, 0x6f,
	0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x45, 0x6d, 0x70,
	0x74, 0x79, 0x1a, 0x18, 0x2e, 0x76, 0x69, 0x6e, 0x74, 0x61, 0x67, 0x65, 0x2e, 0x76, 0x31, 0x2e,
	0x54, 0x69, 0x6d, 0x65, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x10, 0x82, 0xd3,
	0xe4, 0x93, 0x02, 0x0a, 0x12, 0x08, 0x2f, 0x76, 0x31, 0x2f, 0x74, 0x69, 0x6d, 0x65, 0x12, 0x59,
	0x0a, 0x0e, 0x53, 0x74, 0x72, 0x65, 0x61, 0x6d, 0x47, 0x61, 0x6d, 0x65, 0x54, 0x69, 0x6d, 0x65,
	0x12, 0x16, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62,
	0x75, 0x66, 0x2e, 0x45, 0x6d, 0x70, 0x74, 0x79, 0x1a, 0x18, 0x2e, 0x76, 0x69, 0x6e, 0x74, 0x61,
	0x67, 0x65, 0x2e, 0x76, 0x31, 0x2e, 0x54, 0x69, 0x6d, 0x65, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e,
	0x73, 0x65, 0x22, 0x13, 0x82, 0xd3, 0xe4, 0x93, 0x02, 0x0d, 0x12, 0x0b, 0x2f, 0x76, 0x31, 0x2f,
	0x74, 0x69, 0x6d, 0x65, 0x2f, 0x77, 0x73, 0x30, 0x01, 0x12, 0x6c, 0x0a, 0x15, 0x47, 0x65, 0x74,
	0x4f, 0x6e, 0x6c, 0x69, 0x6e, 0x65, 0x50, 0x6c, 0x61, 0x79, 0x65, 0x72, 0x73, 0x43, 0x6f, 0x75,
	0x6e, 0x74, 0x12, 0x16, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74,
	0x6f, 0x62, 0x75, 0x66, 0x2e, 0x45, 0x6d, 0x70, 0x74, 0x79, 0x1a, 0x20, 0x2e, 0x76, 0x69, 0x6e,
	0x74, 0x61, 0x67, 0x65, 0x2e, 0x76, 0x31, 0x2e, 0x50, 0x6c, 0x61, 0x79, 0x65, 0x72, 0x73, 0x43,
	0x6f, 0x75, 0x6e, 0x74, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x19, 0x82, 0xd3,
	0xe4, 0x93, 0x02, 0x13, 0x12, 0x11, 0x2f, 0x76, 0x31, 0x2f, 0x70, 0x6c, 0x61, 0x79, 0x65, 0x72,
	0x73, 0x2f, 0x63, 0x6f, 0x75, 0x6e, 0x74, 0x12, 0x74, 0x0a, 0x18, 0x53, 0x74, 0x72, 0x65, 0x61,
	0x6d, 0x4f, 0x6e, 0x6c, 0x69, 0x6e, 0x65, 0x50, 0x6c, 0x61, 0x79, 0x65, 0x72, 0x73, 0x43, 0x6f,
	0x75, 0x6e, 0x74, 0x12, 0x16, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f,
	0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x45, 0x6d, 0x70, 0x74, 0x79, 0x1a, 0x20, 0x2e, 0x76, 0x69,
	0x6e, 0x74, 0x61, 0x67, 0x65, 0x2e, 0x76, 0x31, 0x2e, 0x50, 0x6c, 0x61, 0x79, 0x65, 0x72, 0x73,
	0x43, 0x6f, 0x75, 0x6e, 0x74, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x1c, 0x82,
	0xd3, 0xe4, 0x93, 0x02, 0x16, 0x12, 0x14, 0x2f, 0x76, 0x31, 0x2f, 0x70, 0x6c, 0x61, 0x79, 0x65,
	0x72, 0x73, 0x2f, 0x63, 0x6f, 0x75, 0x6e, 0x74, 0x2f, 0x77, 0x73, 0x30, 0x01, 0x12, 0x69, 0x0a,
	0x14, 0x47, 0x65, 0x74, 0x4f, 0x6e, 0x6c, 0x69, 0x6e, 0x65, 0x50, 0x6c, 0x61, 0x79, 0x65, 0x72,
	0x73, 0x4c, 0x69, 0x73, 0x74, 0x12, 0x16, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x45, 0x6d, 0x70, 0x74, 0x79, 0x1a, 0x1f, 0x2e,
	0x76, 0x69, 0x6e, 0x74, 0x61, 0x67, 0x65, 0x2e, 0x76, 0x31, 0x2e, 0x50, 0x6c, 0x61, 0x79, 0x65,
	0x72, 0x73, 0x4c, 0x69, 0x73, 0x74, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x18,
	0x82, 0xd3, 0xe4, 0x93, 0x02, 0x12, 0x12, 0x10, 0x2f, 0x76, 0x31, 0x2f, 0x70, 0x6c, 0x61, 0x79,
	0x65, 0x72, 0x73, 0x2f, 0x6c, 0x69, 0x73, 0x74, 0x12, 0x71, 0x0a, 0x17, 0x53, 0x74, 0x72, 0x65,
	0x61, 0x6d, 0x4f, 0x6e, 0x6c, 0x69, 0x6e, 0x65, 0x50, 0x6c, 0x61, 0x79, 0x65, 0x72, 0x73, 0x4c,
	0x69, 0x73, 0x74, 0x12, 0x16, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f,
	0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x45, 0x6d, 0x70, 0x74, 0x79, 0x1a, 0x1f, 0x2e, 0x76, 0x69,
	0x6e, 0x74, 0x61, 0x67, 0x65, 0x2e, 0x76, 0x31, 0x2e, 0x50, 0x6c, 0x61, 0x79, 0x65, 0x72, 0x73,
	0x4c, 0x69, 0x73, 0x74, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x1b, 0x82, 0xd3,
	0xe4, 0x93, 0x02, 0x15, 0x12, 0x13, 0x2f, 0x76, 0x31, 0x2f, 0x70, 0x6c, 0x61, 0x79, 0x65, 0x72,
	0x73, 0x2f, 0x6c, 0x69, 0x73, 0x74, 0x2f, 0x77, 0x73, 0x30, 0x01, 0x12, 0x6b, 0x0a, 0x0e, 0x47,
	0x65, 0x74, 0x50, 0x6c, 0x61, 0x79, 0x65, 0x72, 0x53, 0x74, 0x61, 0x74, 0x73, 0x12, 0x1e, 0x2e,
	0x76, 0x69, 0x6e, 0x74, 0x61, 0x67, 0x65, 0x2e, 0x76, 0x31, 0x2e, 0x50, 0x6c, 0x61, 0x79, 0x65,
	0x72, 0x53, 0x74, 0x61, 0x74, 0x73, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x1f, 0x2e,
	0x76, 0x69, 0x6e, 0x74, 0x61, 0x67, 0x65, 0x2e, 0x76, 0x31, 0x2e, 0x50, 0x6c, 0x61, 0x79, 0x65,
	0x72, 0x53, 0x74, 0x61, 0x74, 0x73, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x18,
	0x82, 0xd3, 0xe4, 0x93, 0x02, 0x12, 0x12, 0x10, 0x2f, 0x76, 0x31, 0x2f, 0x7b, 0x6e, 0x61, 0x6d,
	0x65, 0x7d, 0x2f, 0x73, 0x74, 0x61, 0x74, 0x73, 0x1a, 0x14, 0xca, 0x41, 0x11, 0x61, 0x70, 0x69,
	0x2e, 0x6c, 0x61, 0x73, 0x74, 0x68, 0x65, 0x61, 0x72, 0x74, 0x68, 0x2e, 0x72, 0x75, 0x42, 0x21,
	0x5a, 0x1f, 0x67, 0x69, 0x74, 0x68, 0x75, 0x62, 0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x72, 0x69, 0x70,
	0x6c, 0x73, 0x35, 0x36, 0x2f, 0x76, 0x73, 0x73, 0x65, 0x72, 0x76, 0x69, 0x63, 0x65, 0x2f, 0x76,
	0x31, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
})

var (
	file_proto_v1_vsapi_proto_rawDescOnce sync.Once
	file_proto_v1_vsapi_proto_rawDescData []byte
)

func file_proto_v1_vsapi_proto_rawDescGZIP() []byte {
	file_proto_v1_vsapi_proto_rawDescOnce.Do(func() {
		file_proto_v1_vsapi_proto_rawDescData = protoimpl.X.CompressGZIP(unsafe.Slice(unsafe.StringData(file_proto_v1_vsapi_proto_rawDesc), len(file_proto_v1_vsapi_proto_rawDesc)))
	})
	return file_proto_v1_vsapi_proto_rawDescData
}

var file_proto_v1_vsapi_proto_msgTypes = make([]protoimpl.MessageInfo, 5)
var file_proto_v1_vsapi_proto_goTypes = []any{
	(*PlayerStatsRequest)(nil),   // 0: vintage.v1.PlayerStatsRequest
	(*PlayerStatsResponse)(nil),  // 1: vintage.v1.PlayerStatsResponse
	(*TimeResponse)(nil),         // 2: vintage.v1.TimeResponse
	(*PlayersCountResponse)(nil), // 3: vintage.v1.PlayersCountResponse
	(*PlayersListResponse)(nil),  // 4: vintage.v1.PlayersListResponse
	(*emptypb.Empty)(nil),        // 5: google.protobuf.Empty
}
var file_proto_v1_vsapi_proto_depIdxs = []int32{
	5, // 0: vintage.v1.VintageService.GetGameTime:input_type -> google.protobuf.Empty
	5, // 1: vintage.v1.VintageService.StreamGameTime:input_type -> google.protobuf.Empty
	5, // 2: vintage.v1.VintageService.GetOnlinePlayersCount:input_type -> google.protobuf.Empty
	5, // 3: vintage.v1.VintageService.StreamOnlinePlayersCount:input_type -> google.protobuf.Empty
	5, // 4: vintage.v1.VintageService.GetOnlinePlayersList:input_type -> google.protobuf.Empty
	5, // 5: vintage.v1.VintageService.StreamOnlinePlayersList:input_type -> google.protobuf.Empty
	0, // 6: vintage.v1.VintageService.GetPlayerStats:input_type -> vintage.v1.PlayerStatsRequest
	2, // 7: vintage.v1.VintageService.GetGameTime:output_type -> vintage.v1.TimeResponse
	2, // 8: vintage.v1.VintageService.StreamGameTime:output_type -> vintage.v1.TimeResponse
	3, // 9: vintage.v1.VintageService.GetOnlinePlayersCount:output_type -> vintage.v1.PlayersCountResponse
	3, // 10: vintage.v1.VintageService.StreamOnlinePlayersCount:output_type -> vintage.v1.PlayersCountResponse
	4, // 11: vintage.v1.VintageService.GetOnlinePlayersList:output_type -> vintage.v1.PlayersListResponse
	4, // 12: vintage.v1.VintageService.StreamOnlinePlayersList:output_type -> vintage.v1.PlayersListResponse
	1, // 13: vintage.v1.VintageService.GetPlayerStats:output_type -> vintage.v1.PlayerStatsResponse
	7, // [7:14] is the sub-list for method output_type
	0, // [0:7] is the sub-list for method input_type
	0, // [0:0] is the sub-list for extension type_name
	0, // [0:0] is the sub-list for extension extendee
	0, // [0:0] is the sub-list for field type_name
}

func init() { file_proto_v1_vsapi_proto_init() }
func file_proto_v1_vsapi_proto_init() {
	if File_proto_v1_vsapi_proto != nil {
		return
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: unsafe.Slice(unsafe.StringData(file_proto_v1_vsapi_proto_rawDesc), len(file_proto_v1_vsapi_proto_rawDesc)),
			NumEnums:      0,
			NumMessages:   5,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_proto_v1_vsapi_proto_goTypes,
		DependencyIndexes: file_proto_v1_vsapi_proto_depIdxs,
		MessageInfos:      file_proto_v1_vsapi_proto_msgTypes,
	}.Build()
	File_proto_v1_vsapi_proto = out.File
	file_proto_v1_vsapi_proto_goTypes = nil
	file_proto_v1_vsapi_proto_depIdxs = nil
}
