// @ts-check

/** @type {import('@docusaurus/types').Config} */
const config = {
  title: 'composed',
  tagline: 'Compose anything into a Docker Compose file',
  favicon: 'img/favicon.ico',

  url: 'https://docker-x.github.io',
  baseUrl: '/composed/',

  organizationName: 'docker-x',
  projectName: 'composed',
  trailingSlash: false,

  onBrokenLinks: 'throw',

  markdown: {
    hooks: {
      onBrokenMarkdownLinks: 'warn',
    },
  },

  i18n: {
    defaultLocale: 'en',
    locales: ['en'],
  },

  presets: [
    [
      'classic',
      /** @type {import('@docusaurus/preset-classic').Options} */
      ({
        docs: {
          routeBasePath: '/',
          sidebarPath: './sidebars.js',
          editUrl: 'https://github.com/docker-x/composed/tree/main/website/',
        },
        blog: false,
        theme: {
          customCss: './src/css/custom.css',
        },
      }),
    ],
  ],

  themeConfig:
    /** @type {import('@docusaurus/preset-classic').ThemeConfig} */
    ({
      navbar: {
        title: 'composed',
        items: [
          {
            type: 'docSidebar',
            sidebarId: 'docs',
            position: 'left',
            label: 'Docs',
          },
          {
            href: 'https://github.com/docker-x/composed',
            label: 'GitHub',
            position: 'right',
          },
        ],
      },
      footer: {
        style: 'dark',
        links: [
          {
            title: 'Docs',
            items: [
              { label: 'Getting Started', to: '/getting-started/installation' },
              { label: 'CLI Reference', to: '/cli/init' },
            ],
          },
          {
            title: 'More',
            items: [
              { label: 'GitHub', href: 'https://github.com/docker-x/composed' },
              { label: 'Docker eXtra', href: 'https://github.com/docker-x' },
            ],
          },
        ],
        copyright: `Copyright ${new Date().getFullYear()} Docker eXtra. Built with Docusaurus.`,
      },
      prism: {
        additionalLanguages: ['bash', 'yaml', 'go'],
      },
    }),
};

export default config;
