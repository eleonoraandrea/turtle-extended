.PHONY: build demo backtest compare download walk montecarlo test clean

BIN=atps

build:
	go build -o ./$(BIN) ./cmd/atps

build-live:
	go build -tags live -o ./$(BIN)-live ./cmd/atps

tui: build
	./$(BIN) tui

demo: build
	./$(BIN) generate-demo
	./$(BIN) demo
	./$(BIN) compare --out reports/comparison.html
	@echo "demo reports in ./reports/"

backtest: build
	./$(BIN) backtest --variant D --symbol BTCUSDT

compare: build
	./$(BIN) compare

download:
	./$(BIN) download --symbol BTCUSDT --interval 4h --funding=true --oi=true
	./$(BIN) download --symbol ETHUSDT --interval 4h --funding=true --oi=true
	./$(BIN) download --symbol SOLUSDT --interval 4h --funding=true --oi=true

walk:
	./$(BIN) walk-forward --symbol BTCUSDT --variant D

montecarlo:
	./$(BIN) montecarlo --symbol BTCUSDT --variant D --runs 2000

test:
	go test ./... -count=1

clean:
	rm -f ./$(BIN) ./$(BIN)-live
	rm -rf reports/*.html reports/*.json

install-deps:
	go mod tidy
	go mod download

report-serve:
	python3 -m http.server 8000 --directory reports

