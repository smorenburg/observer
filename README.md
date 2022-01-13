# observer

https://hub.docker.com/repository/docker/smorenburg/observer

```bash
docker build --platform linux/amd64 -t smorenburg/observer:tagname .
docker push smorenburg/observer:tagname
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
export HOST=localhost:8080
for i in {1..30}; do
  curl "$HOST/random-delay"
done
```

```bash
# 30 requests with a random error.
export HOST=localhost:8080
for i in {1..30}; do
  curl "$HOST/random-error"
done
```
