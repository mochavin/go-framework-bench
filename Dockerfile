# One image per framework: docker build --build-arg FRAMEWORK=gin .
FROM golang:1.26-alpine AS build
ARG FRAMEWORK
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /server ./cmd/${FRAMEWORK}

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /server /server
EXPOSE 8000
ENTRYPOINT ["/server"]
