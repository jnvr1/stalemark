# Observability

```bash
docker exec prometheus wget -qO- http://localhost:9090/-/reload
kubectl exec api -- curl localhost:8080/health
docker run --rm curlimages/curl \
  curl http://localhost:6060/
curl http://localhost:7777/
```
