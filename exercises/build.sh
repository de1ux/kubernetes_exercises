go get -v github.com/tools/godep

go get -v k8s.io/client-go/...
pushd $GOPATH/src/k8s.io/client-go
git checkout v6.0.0
godep restore ./...
popd