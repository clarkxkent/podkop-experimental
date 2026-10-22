export function getClashUIUrl(): string {
  const { hostname } = window.location;

  return `http://${hostname}:9090/ui`;
}
