FROM gcr.io/distroless/static-debian12:nonroot
ARG TARGETPLATFORM
COPY $TARGETPLATFORM/stalemark /stalemark
WORKDIR /work
ENTRYPOINT ["/stalemark"]
