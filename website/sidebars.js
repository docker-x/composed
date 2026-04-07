/** @type {import('@docusaurus/plugin-content-docs').SidebarsConfig} */
const sidebars = {
  docs: [
    'intro',
    {
      type: 'category',
      label: 'Getting Started',
      items: [
        'getting-started/installation',
        'getting-started/quick-start',
      ],
    },
    {
      type: 'category',
      label: 'Guide',
      items: [
        'guide/config-file',
        'guide/extensions',
        'guide/helm-values',
        'guide/translation-rules',
      ],
    },
    {
      type: 'category',
      label: 'CLI Reference',
      items: [
        'cli/init',
        'cli/add',
        'cli/build',
        'cli/up-down',
      ],
    },
    {
      type: 'category',
      label: 'Internals',
      items: [
        'architecture/overview',
      ],
    },
  ],
};

export default sidebars;
