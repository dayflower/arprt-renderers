FROM python:3.13-alpine@sha256:2d9aefe2fef018a7eb2c13064c89c71929800fd2e5dccdbf52ea5da5bb8d929a
ARG VERSION=dev
ARG REVISION=unknown
LABEL org.opencontainers.image.title="ARPRT Picsum renderer" \
      org.opencontainers.image.description="Photo Job renderer maintained by the independent, unofficial ARPRT project; not affiliated with or endorsed by TRMNL." \
      org.opencontainers.image.source="https://github.com/dayflower/arprt-renderers" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${REVISION}" \
      org.opencontainers.image.licenses="MIT"
WORKDIR /app
COPY renderers/picsum/requirements.txt /app/requirements.txt
RUN pip install --no-cache-dir --only-binary=:all: -r requirements.txt
COPY renderers/picsum/renderer.py /app/renderer.py
COPY LICENSE /usr/share/licenses/arprt-renderer-picsum/LICENSE
ENV PYTHONDONTWRITEBYTECODE=1
USER 1000:1000
ENTRYPOINT ["python3", "/app/renderer.py"]
