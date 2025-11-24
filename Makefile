CSV_SRC=Test_Import_CSV-Datei_2025
CSV_SRC_FILE=$(CSV_SRC).csv
CSV_RESULT_FILE=result_$(CSV_SRC).csv

build:
	go build -o csvget main.go

build-no-http2:
	GODEBUG=http2client=0 go build -o csvget main.go

run:
	./csvget \
		-parallel 20 \
		-csv-file $(CSV_SRC_FILE) \
		-result-file $(CSV_RESULT_FILE)

clean:
	rm -f csvget

