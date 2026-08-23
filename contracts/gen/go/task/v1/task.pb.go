package taskv1

import (
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

type Task struct {
	ID          string `json:"id"`
	OwnerID     string `json:"owner_id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Status      string `json:"status"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}
type ListTasksRequest struct{}
type ListTasksResponse struct {
	Tasks []*Task `json:"tasks"`
}
type GetTaskRequest struct {
	ID string `json:"id"`
}
type CreateTaskRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}
type UpdateTaskRequest struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Status      string `json:"status"`
}
type DeleteTaskRequest struct {
	ID string `json:"id"`
}
type Empty struct{}
type TaskServiceClient interface {
	ListTasks(context.Context, *ListTasksRequest, ...grpc.CallOption) (*ListTasksResponse, error)
	GetTask(context.Context, *GetTaskRequest, ...grpc.CallOption) (*Task, error)
	CreateTask(context.Context, *CreateTaskRequest, ...grpc.CallOption) (*Task, error)
	UpdateTask(context.Context, *UpdateTaskRequest, ...grpc.CallOption) (*Task, error)
	DeleteTask(context.Context, *DeleteTaskRequest, ...grpc.CallOption) (*Empty, error)
}
type taskServiceClient struct{ cc grpc.ClientConnInterface }

func NewTaskServiceClient(cc grpc.ClientConnInterface) TaskServiceClient {
	return &taskServiceClient{cc}
}
func (c *taskServiceClient) ListTasks(x context.Context, in *ListTasksRequest, o ...grpc.CallOption) (*ListTasksResponse, error) {
	out := new(ListTasksResponse)
	e := c.cc.Invoke(x, "/task.v1.TaskService/ListTasks", in, out, o...)
	return out, e
}
func (c *taskServiceClient) GetTask(x context.Context, in *GetTaskRequest, o ...grpc.CallOption) (*Task, error) {
	out := new(Task)
	e := c.cc.Invoke(x, "/task.v1.TaskService/GetTask", in, out, o...)
	return out, e
}
func (c *taskServiceClient) CreateTask(x context.Context, in *CreateTaskRequest, o ...grpc.CallOption) (*Task, error) {
	out := new(Task)
	e := c.cc.Invoke(x, "/task.v1.TaskService/CreateTask", in, out, o...)
	return out, e
}
func (c *taskServiceClient) UpdateTask(x context.Context, in *UpdateTaskRequest, o ...grpc.CallOption) (*Task, error) {
	out := new(Task)
	e := c.cc.Invoke(x, "/task.v1.TaskService/UpdateTask", in, out, o...)
	return out, e
}
func (c *taskServiceClient) DeleteTask(x context.Context, in *DeleteTaskRequest, o ...grpc.CallOption) (*Empty, error) {
	out := new(Empty)
	e := c.cc.Invoke(x, "/task.v1.TaskService/DeleteTask", in, out, o...)
	return out, e
}

type TaskServiceServer interface {
	ListTasks(context.Context, *ListTasksRequest) (*ListTasksResponse, error)
	GetTask(context.Context, *GetTaskRequest) (*Task, error)
	CreateTask(context.Context, *CreateTaskRequest) (*Task, error)
	UpdateTask(context.Context, *UpdateTaskRequest) (*Task, error)
	DeleteTask(context.Context, *DeleteTaskRequest) (*Empty, error)
	mustEmbedUnimplementedTaskServiceServer()
}
type UnimplementedTaskServiceServer struct{}

func (UnimplementedTaskServiceServer) ListTasks(context.Context, *ListTasksRequest) (*ListTasksResponse, error) {
	return nil, status.Error(codes.Unimplemented, "method ListTasks not implemented")
}
func (UnimplementedTaskServiceServer) GetTask(context.Context, *GetTaskRequest) (*Task, error) {
	return nil, status.Error(codes.Unimplemented, "method GetTask not implemented")
}
func (UnimplementedTaskServiceServer) CreateTask(context.Context, *CreateTaskRequest) (*Task, error) {
	return nil, status.Error(codes.Unimplemented, "method CreateTask not implemented")
}
func (UnimplementedTaskServiceServer) UpdateTask(context.Context, *UpdateTaskRequest) (*Task, error) {
	return nil, status.Error(codes.Unimplemented, "method UpdateTask not implemented")
}
func (UnimplementedTaskServiceServer) DeleteTask(context.Context, *DeleteTaskRequest) (*Empty, error) {
	return nil, status.Error(codes.Unimplemented, "method DeleteTask not implemented")
}
func (UnimplementedTaskServiceServer) mustEmbedUnimplementedTaskServiceServer() {}
func RegisterTaskServiceServer(s grpc.ServiceRegistrar, x TaskServiceServer) {
	s.RegisterService(&TaskService_ServiceDesc, x)
}
func h(s interface{}, ctx context.Context, d func(interface{}) error, i grpc.UnaryServerInterceptor, in interface{}, method string, call func(interface{}) (interface{}, error)) (interface{}, error) {
	if err := d(in); err != nil {
		return nil, err
	}
	if i == nil {
		return call(in)
	}
	info := &grpc.UnaryServerInfo{Server: s, FullMethod: method}
	return i(ctx, in, info, func(c context.Context, r interface{}) (interface{}, error) { return call(r) })
}
func _l(s interface{}, c context.Context, d func(interface{}) error, i grpc.UnaryServerInterceptor) (interface{}, error) {
	return h(s, c, d, i, new(ListTasksRequest), "/task.v1.TaskService/ListTasks", func(r interface{}) (interface{}, error) {
		return s.(TaskServiceServer).ListTasks(c, r.(*ListTasksRequest))
	})
}
func _g(s interface{}, c context.Context, d func(interface{}) error, i grpc.UnaryServerInterceptor) (interface{}, error) {
	return h(s, c, d, i, new(GetTaskRequest), "/task.v1.TaskService/GetTask", func(r interface{}) (interface{}, error) { return s.(TaskServiceServer).GetTask(c, r.(*GetTaskRequest)) })
}
func _c(s interface{}, c context.Context, d func(interface{}) error, i grpc.UnaryServerInterceptor) (interface{}, error) {
	return h(s, c, d, i, new(CreateTaskRequest), "/task.v1.TaskService/CreateTask", func(r interface{}) (interface{}, error) {
		return s.(TaskServiceServer).CreateTask(c, r.(*CreateTaskRequest))
	})
}
func _u(s interface{}, c context.Context, d func(interface{}) error, i grpc.UnaryServerInterceptor) (interface{}, error) {
	return h(s, c, d, i, new(UpdateTaskRequest), "/task.v1.TaskService/UpdateTask", func(r interface{}) (interface{}, error) {
		return s.(TaskServiceServer).UpdateTask(c, r.(*UpdateTaskRequest))
	})
}
func _d(s interface{}, c context.Context, d func(interface{}) error, i grpc.UnaryServerInterceptor) (interface{}, error) {
	return h(s, c, d, i, new(DeleteTaskRequest), "/task.v1.TaskService/DeleteTask", func(r interface{}) (interface{}, error) {
		return s.(TaskServiceServer).DeleteTask(c, r.(*DeleteTaskRequest))
	})
}

var TaskService_ServiceDesc = grpc.ServiceDesc{ServiceName: "task.v1.TaskService", HandlerType: (*TaskServiceServer)(nil), Methods: []grpc.MethodDesc{{MethodName: "ListTasks", Handler: _l}, {MethodName: "GetTask", Handler: _g}, {MethodName: "CreateTask", Handler: _c}, {MethodName: "UpdateTask", Handler: _u}, {MethodName: "DeleteTask", Handler: _d}}}
