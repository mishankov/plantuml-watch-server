FROM golang:1.26 AS buildgo
WORKDIR /app
COPY . .
RUN CGO_ENABLED=0 go build .

FROM plantuml/plantuml:latest
WORKDIR /opt
COPY --from=buildgo /app/plantuml-watch-server /opt/plantuml-watch-server
EXPOSE 8080
ENTRYPOINT [ "./plantuml-watch-server", "run", "-input=/input", "-output=/output" ]
