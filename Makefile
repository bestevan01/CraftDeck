.PHONY: web build run clean

web:
	npm --prefix web install
	npm --prefix web run build

build: web
	mkdir -p build
	go build -buildvcs=false -o build/craftdeckd ./cmd/craftdeckd

run: build
	./build/craftdeckd

clean:
	rm -rf build web/build web/.svelte-kit
