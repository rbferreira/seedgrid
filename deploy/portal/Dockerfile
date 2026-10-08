# SeedGrid Portal — runtime image (COPY ONLY, no RUN; see control-plane/Dockerfile.runtime).
# The static build (portal/dist) is generated beforehand by scripts/build-portal.sh (docker run node).
# Build context: repository root.
FROM nginx:1.27-alpine
COPY deploy/portal/nginx.conf /etc/nginx/conf.d/default.conf
COPY portal/dist /usr/share/nginx/html
EXPOSE 80
