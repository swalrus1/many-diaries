build:
    cd src && go build -o ../build/many-diaries ./cmd/many-diaries

run: build
    ./build/many-diaries
