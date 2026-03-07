FROM gcr.io/distroless/base-debian12:nonroot
COPY mcp-filepuff /usr/local/bin/mcp-filepuff
ENTRYPOINT ["mcp-filepuff"]
