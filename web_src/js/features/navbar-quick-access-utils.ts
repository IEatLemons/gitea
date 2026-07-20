export function buildNavbarRepoSearchUrl(appSubUrl: string, uid: number, query: string, limit: number) {
  const params = new URLSearchParams({
    sort: 'updated',
    order: 'desc',
    uid: String(uid),
    q: query,
    page: '1',
    limit: String(limit),
    archived: 'false',
  });
  return `${appSubUrl}/repo/search?${params.toString()}`;
}

export function buildNavbarOrganizationsUrl(appSubUrl: string, query: string, limit: number) {
  const params = new URLSearchParams({
    q: query,
    limit: String(limit),
  });
  return `${appSubUrl}/-/navbar/organizations?${params.toString()}`;
}
