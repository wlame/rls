FROM alpine:latest
RUN apk add --no-cache bash curl jq
COPY scripts/ /scripts/
RUN chmod +x /scripts/*.sh
CMD ["/scripts/run-all.sh"]
