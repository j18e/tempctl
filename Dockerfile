FROM alpine:3.10

ADD tempctl .

ENTRYPOINT ["./tempctl"]
