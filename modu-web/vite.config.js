import { resolve } from 'node:path';
import { defineConfig } from 'vite';

export default defineConfig({
  build: {
    rollupOptions: {
      input: {
        index: resolve(__dirname, 'index.html'),
        notFound: resolve(__dirname, '404.html'),
        docs: resolve(__dirname, 'docs.html'),
        terms: resolve(__dirname, 'terms.html'),
        privacy: resolve(__dirname, 'privacy.html'),
        pricing: resolve(__dirname, 'pricing.html'),
        englishIndex: resolve(__dirname, 'en/index.html'),
        englishDocs: resolve(__dirname, 'en/docs.html'),
        englishTerms: resolve(__dirname, 'en/terms.html'),
        englishPrivacy: resolve(__dirname, 'en/privacy.html'),
        englishPricing: resolve(__dirname, 'en/pricing.html'),
        docsCli: resolve(__dirname, 'docs/cli.html'),
        docsModels: resolve(__dirname, 'docs/models.html'),
        englishDocsCli: resolve(__dirname, 'en/docs/cli.html'),
        englishDocsModels: resolve(__dirname, 'en/docs/models.html')
      }
    }
  }
});
