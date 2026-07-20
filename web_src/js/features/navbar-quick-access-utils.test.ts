import {buildNavbarOrganizationsUrl, buildNavbarRepoSearchUrl} from './navbar-quick-access-utils.ts';

test('buildNavbarRepoSearchUrl', () => {
  expect(buildNavbarRepoSearchUrl('/sub', 4, 'xkg/unipie private', 8)).toEqual('/sub/repo/search?sort=updated&order=desc&uid=4&q=xkg%2Funipie+private&page=1&limit=8&archived=false');
});

test('buildNavbarOrganizationsUrl', () => {
  expect(buildNavbarOrganizationsUrl('', 'org 3', 8)).toEqual('/-/navbar/organizations?q=org+3&limit=8');
});
