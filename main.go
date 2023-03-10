package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	pb "grpc_gateway_media_example/pb"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection"
)

type ServerInterface interface {
	Serve(net.Listener) error
}

func launchServer(server ServerInterface, port int) {
	conn, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		fmt.Printf("failed to listen: %v\n", err)
		os.Exit(1)
	}
	err = server.Serve(conn)

	if err != nil {
		fmt.Printf("Serve on port %d: %v\n", port, err)
		os.Exit(1)
	}
}

func GetFormFile(r *http.Request, name string) ([]byte, error) {
	file, _, err := r.FormFile(name)
	if err != nil {
		return nil, fmt.Errorf("not found")
	}
	defer file.Close()
	buf := bytes.Buffer{}
	io.Copy(&buf, file)
	if err != nil {
		return nil, fmt.Errorf("error while reading form file")
	}
	return buf.Bytes(), nil
}

func EncodeAndSubstitute(jsonData map[string]interface{}, r *http.Request, res *map[string]interface{}) error {
	for name, val := range jsonData {
		if inner, isObject := val.(map[string]interface{}); isObject {
			(*res)[name] = map[string]interface{}{}
			resInner, _ := (*res)[name].(map[string]interface{})
			err := EncodeAndSubstitute(inner, r, &resInner)
			if err != nil {
				return err
			}
		} else if valStr, ok := val.(string); ok && strings.HasPrefix(valStr, "$") {
			data, err := GetFormFile(r, valStr)
			if err != nil {
				return fmt.Errorf("can not get file %s", val.(string))
			}
			(*res)[name] = base64.StdEncoding.EncodeToString(data)
		} else {
			(*res)[name] = val
		}
	}
	return nil
}

func createRequestFromMultiPart(r *http.Request) (*http.Request, error) {
	json_data, err := GetFormFile(r, "data")
	if err != nil {
		fmt.Printf("%v", err)
		return nil, err
	}
	rawJson := json.RawMessage(json_data)
	var decoded map[string]interface{}
	encodedAndSubstituted := map[string]interface{}{}
	json.Unmarshal(rawJson, &decoded)
	err = EncodeAndSubstitute(decoded, r, &encodedAndSubstituted)
	str, _ := json.Marshal(encodedAndSubstituted)
	if err != nil {
		return nil, err
	}
	reader := bytes.NewReader(str)
	newR, err := http.NewRequest(http.MethodPost, r.URL.String(), reader)
	if err != nil {
		return nil, err
	}
	return newR, nil
}

func grpcHandlerFunc(grpcServer *grpc.Server, otherHandler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor == 2 && strings.Contains(r.Header.Get("Content-Type"), "application/grpc") {
			grpcServer.ServeHTTP(w, r)
		} else {
			if strings.Contains(r.Header.Get("Content-Type"), "multipart/form-data") {
				newR, err := createRequestFromMultiPart(r)
				if err != nil {
					w.WriteHeader(400)
					return
				}
				otherHandler.ServeHTTP(w, newR)
			} else {
				otherHandler.ServeHTTP(w, r)
			}
		}
	})
}

type EchoService struct {
	pb.UnimplementedEchoServiceServer
}

func (service *EchoService) Echo(ctx context.Context, in *pb.EchoMessage) (*pb.EchoMessage, error) {
	var res *pb.EchoMessage
	handler := func() {
		time.Sleep(time.Second)
		res = in
	}
	Process(handler, "echo")
	return res, nil
}

func Process(callback func(), label string) {
	fmt.Printf("got request of type %s\n", label)
	start := time.Now()
	callback()
	fmt.Printf("it took %f seconds to process %s\n", time.Since(start).Seconds(), label)
}

func main() {
	grpcServer := grpc.NewServer()
	mux := http.NewServeMux()
	pb.RegisterEchoServiceServer(grpcServer, &EchoService{})
	reflection.Register(grpcServer)
	ctx := context.Background()
	gwmux := runtime.NewServeMux()
	err := pb.RegisterEchoServiceHandlerFromEndpoint(ctx, gwmux, "localhost:9090", []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())})
	if err != nil {
		fmt.Printf("%v\n", err)
		os.Exit(1)
	}
	mux.Handle("/v1/echo", gwmux)
	// launch grpc server
	go launchServer(grpcServer, 9090)

	// launch http server redirecting to grpc
	srv := &http.Server{
		Addr:    "0.0.0.0:8080",
		Handler: grpcHandlerFunc(grpcServer, mux),
	}
	go launchServer(srv, 8080)

	for {
		time.Sleep(time.Second)
	}
}
