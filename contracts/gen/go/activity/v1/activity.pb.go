package activityv1

import (
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

type Activity struct {
	ID         string `json:"id"`
	EventID    string `json:"event_id"`
	EventType  string `json:"event_type"`
	TaskID     string `json:"task_id"`
	ActorID    string `json:"actor_id"`
	OccurredAt string `json:"occurred_at"`
}
type ListActivitiesRequest struct{}
type ListActivitiesResponse struct {
	Activities []*Activity `json:"activities"`
}
type ActivityServiceClient interface {
	ListActivities(context.Context, *ListActivitiesRequest, ...grpc.CallOption) (*ListActivitiesResponse, error)
}
type activityServiceClient struct{ cc grpc.ClientConnInterface }

func NewActivityServiceClient(cc grpc.ClientConnInterface) ActivityServiceClient {
	return &activityServiceClient{cc}
}
func (c *activityServiceClient) ListActivities(x context.Context, in *ListActivitiesRequest, o ...grpc.CallOption) (*ListActivitiesResponse, error) {
	out := new(ListActivitiesResponse)
	e := c.cc.Invoke(x, "/activity.v1.ActivityService/ListActivities", in, out, o...)
	return out, e
}

type ActivityServiceServer interface {
	ListActivities(context.Context, *ListActivitiesRequest) (*ListActivitiesResponse, error)
	mustEmbedUnimplementedActivityServiceServer()
}
type UnimplementedActivityServiceServer struct{}

func (UnimplementedActivityServiceServer) ListActivities(context.Context, *ListActivitiesRequest) (*ListActivitiesResponse, error) {
	return nil, status.Error(codes.Unimplemented, "method not implemented")
}
func (UnimplementedActivityServiceServer) mustEmbedUnimplementedActivityServiceServer() {}
func RegisterActivityServiceServer(s grpc.ServiceRegistrar, x ActivityServiceServer) {
	s.RegisterService(&ActivityService_ServiceDesc, x)
}
func _l(s interface{}, c context.Context, d func(interface{}) error, i grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ListActivitiesRequest)
	if err := d(in); err != nil {
		return nil, err
	}
	if i == nil {
		return s.(ActivityServiceServer).ListActivities(c, in)
	}
	info := &grpc.UnaryServerInfo{Server: s, FullMethod: "/activity.v1.ActivityService/ListActivities"}
	return i(c, in, info, func(c context.Context, r interface{}) (interface{}, error) {
		return s.(ActivityServiceServer).ListActivities(c, r.(*ListActivitiesRequest))
	})
}

var ActivityService_ServiceDesc = grpc.ServiceDesc{ServiceName: "activity.v1.ActivityService", HandlerType: (*ActivityServiceServer)(nil), Methods: []grpc.MethodDesc{{MethodName: "ListActivities", Handler: _l}}}
