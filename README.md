# demo-app

```bash
docker build --platform linux/amd64 -t smorenburg/demo-app:tagname .
docker push smorenburg/demo-app:tagname
```

```bash
# 30 requests with a random delay in ms.
for i in {1..30}; do
  curl "http://localhost:8080/random-delay"
done
```

```bash
# 30 requests with a random error.
for i in {1..30}; do
  curl "http://localhost:8080/random-error"
done
```
