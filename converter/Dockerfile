# Document conversion sidecar: headless LibreOffice behind a tiny HTTP API.
#
# This lives in its own image on purpose. The backend runtime is alpine:3.21
# with ca-certificates and tzdata (backend/Dockerfile) — roughly 40 MB — and
# LibreOffice would add close to a gigabyte to *every* backend deploy, including
# the ones that never touch a document. Tessera treats this service as optional
# (CONVERTER_URL empty or unreachable = import/export says so and the rest of
# the Documents section keeps working), so the weight stays where it is used.
FROM debian:bookworm-slim

# libreoffice-writer-nogui is the word-processing component only: Calc and
# Impress handle formats the block document cannot represent anyway.
#
# The fonts are not incidental. A LibreOffice image without them renders
# Cyrillic as boxes in exported PDFs — the text is in the file and looks
# destroyed on screen, which is the worst kind of failure because it survives
# every automated check that only looks at bytes. DejaVu and Liberation cover
# Cyrillic and give metric-compatible stand-ins for Arial/Times/Courier, which
# is what an imported .docx will ask for.
RUN apt-get update && apt-get install -y --no-install-recommends \
      libreoffice-writer-nogui \
      libreoffice-java-common \
      python3 \
      fonts-dejavu-core \
      fonts-liberation2 \
      fonts-noto-core \
 && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY server.py .

# Warm the LibreOffice profile at build time. The first soffice run of a fresh
# container spends several seconds creating ~/.config/libreoffice, and that cost
# would otherwise land on the first user who imports a document.
RUN soffice --headless --terminate_after_init >/dev/null 2>&1 || true

ENV CONVERTER_PORT=3000
EXPOSE 3000

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD python3 -c "import urllib.request,sys; sys.exit(0 if urllib.request.urlopen('http://127.0.0.1:3000/health', timeout=3).status==200 else 1)"

CMD ["python3", "server.py"]
