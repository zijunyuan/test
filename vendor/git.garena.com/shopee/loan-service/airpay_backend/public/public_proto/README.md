### 引用
> import "git.garena.com/shopee/loan-service/airpay_backend/public/public_proto/common.proto";

### 编译
> protoc -I "./" -I "/data/code/kredit/go/src/" --go_out=plugins=grpc:../protobuf/risk risk.proto

> 需要把common.proto下载到本地，其中两个-I是分别导入risk和common pb文件目录

### grpcui使用
- 先生成protoset文件
> protoc --descriptor_set_out=risk.protoset -I "/data/code/kredit/go/src/" -I "./" --include_imports risk.proto
- grpcui 指定protoset文件
> grpcui -v -protoset risk.protoset -plaintext localhost:31106
