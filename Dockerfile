FROM golang:1.17.1 AS build
WORKDIR /workspace
COPY . /workspace
RUN go build -o ./dist/stonks ./cmd/stonks

FROM golang:1.17.1

COPY --from=build /workspace/dist/stonks /bin/stonks
CMD ["/bin/stonks"]