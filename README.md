## Envoy Control Plane

A simple envoy control plane written in Go.

#### Static Configuration

Run
```
envoy -c google.yaml
```

Now if you try to browse `localhost:10000`, it will redirect to `google.com`

#### Dynamic Configuration

Do the same task with dynamic configuratin and xds server.

Run these two commands in two separate terminal sessions
```
envoy -c dynamic.yaml
go run main.go
```

