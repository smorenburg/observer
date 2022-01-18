# Observer

https://hub.docker.com/repository/docker/smorenburg/observer

```bash
docker build --platform linux/amd64 -t smorenburg/observer:tagname .
docker push smorenburg/observer:tagname
```

```bash
docker run -d -p 27017:27017 --name mongodb \
      -e MONGO_INITDB_ROOT_USERNAME=admin \
      -e MONGO_INITDB_ROOT_PASSWORD=password \
      mongo
```

```bash
export PROJECT_ID=project_id
export KEY_ID=projects/$PROJECT_ID/locations/europe-west4/keyRings/cluster-01/cryptoKeys/observer
sops --encrypt \
  --encrypted-regex '^(data|stringData)$' \
  --gcp-kms $KEY_ID observer-secrets.yaml > observer-secrets.encrypted.yaml
```

```bash
sops --decrypt observer-secrets.encrypted.yaml | kubectl apply -f -
```

```bash
# Verify the exporter metrics path.
POD_NAME=observer-db-0
NAMESPACE=observer
PORT_NUMBER=9090
METRICS_PATH=/metrics
kubectl get --raw /api/v1/namespaces/$NAMESPACE/pods/$POD_NAME:$PORT_NUMBER/proxy/$METRICS_PATH
```

```bash
# 1000 requests
export HOST=http://localhost:8080
for i in {1..1000}; do
  curl "$HOST"
done
```

```bash
# 30 requests with a random delay in ms.
export HOST=http://localhost:8080
for i in {1..30}; do
  curl "$HOST/?latency=random"
done
```

```bash
# 30 requests with a random HTTP server error 500.
export HOST=http://localhost:8080
for i in {1..30}; do
  curl "$HOST/?error=random"
done
```
