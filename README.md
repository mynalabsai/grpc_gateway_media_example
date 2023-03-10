# gRPC-gateway for mediafiles

Example of service thar provides both HTTP API and gRPC, common logic wrapper, and allows using mediafiles.

## Usage

Launch
```bash
go run main.go
```

Check plain request
```bash
curl localhost:8080/v1/echo  -H "Content-Type: applicaton/json" -d@request_plain.json
```

Use macros in request
```bash
curl localhost:8080/v1/echo  -H "Content-Type: multipart/form-data" -F 'data=@request_macros.json' -F '$neiro=@Neiro.png'
```

You can also send gRPC requests to `localhost:9090`. We use [grpcurl](https://github.com/fullstorydev/grpcurl).
```bash
grpcurl --plaintext -d @ localhost:9090  echoproto.EchoService.Echo < request_plain.json
```

