FROM scratch
COPY lazyaws /lazyaws
ENTRYPOINT ["/lazyaws"]
