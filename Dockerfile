FROM gcr.io/distroless/base
COPY /gopssh /gopssh
ENTRYPOINT ["/gopssh"]
