.PHONY: build run gateway backend1 backend2 backend3 test clean client

build:
	go build -o api-gateway .

run: build
	./api-gateway -mode gateway -port 8080 -backends http://localhost:8081,http://localhost:8082

gateway: build
	./api-gateway -mode gateway -port 8080 -backends http://localhost:8081,http://localhost:8082

backend1: build
	./api-gateway -mode backend -port 8081 -name "Backend-1"

backend2: build
	./api-gateway -mode backend -port 8082 -name "Backend-2"

backend3: build
	./api-gateway -mode backend -port 8083 -name "Backend-3"

test: build
	@echo "Testing gateway health..."
	@go run . -mode client -cmd health -endpoint http://localhost:8080
	
	@echo "\nTesting echo endpoint..."
	@go run . -mode client -cmd echo -endpoint http://localhost:8080 -count 3
	
	@echo "\nTesting user endpoint with load balancing..."
	@go run . -mode client -cmd user -endpoint http://localhost:8080 -count 5
	
	@echo "\nTesting authentication..."
	@go run . -mode client -cmd auth -endpoint http://localhost:8080
	
	@echo "\nTesting data endpoint..."
	@go run . -mode client -cmd data -endpoint http://localhost:8080 -count 2

client:
	go run . -mode client -cmd $(CMD) -endpoint http://localhost:8080 -count $(COUNT)

logs:
	tail -f gateway.log | jq .

clean:
	rm -f api-gateway gateway.log
	rm -rf .git

fmt:
	go fmt ./...

all: clean build test
