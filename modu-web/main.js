const root = document.documentElement;
const themeButtons = document.querySelectorAll('.theme-toggle');
const languageButtons = document.querySelectorAll('.language-toggle');
const mobileMenuButton = document.querySelector('.mobile-menu-button');
const navMenu = document.querySelector('.nav-menu');
const toast = document.querySelector('.copy-toast');

const savedTheme = localStorage.getItem('modu-theme');
if (savedTheme === 'dark') {
  root.dataset.theme = 'dark';
}

function syncThemeButtons() {
  const isDark = root.dataset.theme === 'dark';
  themeButtons.forEach((button) => {
    button.setAttribute('aria-label', isDark ? '切换浅色模式' : '切换深色模式');
    const icon = button.querySelector('i');
    if (icon) {
      icon.className = isDark ? 'ti ti-sun' : 'ti ti-moon';
    }
  });
}

syncThemeButtons();

themeButtons.forEach((button) => {
  button.addEventListener('click', () => {
    const nextTheme = root.dataset.theme === 'dark' ? 'light' : 'dark';
    if (nextTheme === 'light') {
      delete root.dataset.theme;
    } else {
      root.dataset.theme = 'dark';
    }
    localStorage.setItem('modu-theme', nextTheme);
    syncThemeButtons();
  });
});

if (mobileMenuButton && navMenu) {
  mobileMenuButton.addEventListener('click', () => {
    const open = navMenu.classList.toggle('open');
    mobileMenuButton.setAttribute('aria-expanded', String(open));
    mobileMenuButton.setAttribute('aria-label', open ? '关闭导航' : '打开导航');
    const icon = mobileMenuButton.querySelector('i');
    if (icon) {
      icon.className = open ? 'ti ti-x' : 'ti ti-menu-2';
    }
  });

  navMenu.querySelectorAll('a').forEach((link) => {
    link.addEventListener('click', () => {
      navMenu.classList.remove('open');
      mobileMenuButton.setAttribute('aria-expanded', 'false');
    });
  });
}

let toastTimer;
function showToast(message) {
  if (!toast) return;
  toast.textContent = message;
  toast.classList.add('visible');
  window.clearTimeout(toastTimer);
  toastTimer = window.setTimeout(() => toast.classList.remove('visible'), 1800);
}

document.querySelectorAll('[data-copy]').forEach((button) => {
  button.addEventListener('click', async () => {
    const text = button.dataset.copy.replace(/\\n/g, '\n');
    const label = button.querySelector('[data-copy-label]');
    try {
      await navigator.clipboard.writeText(text);
      button.classList.add('copied');
      if (label) label.textContent = '已复制';
      showToast('已复制到剪贴板');
      window.setTimeout(() => {
        button.classList.remove('copied');
        if (label) label.textContent = '复制';
      }, 1800);
    } catch {
      showToast('复制失败，请手动选择命令');
    }
  });
});

const originalText = new WeakMap();
document.querySelectorAll('[data-en]').forEach((element) => {
  originalText.set(element, element.innerHTML);
});

function applyLanguage(language) {
  root.lang = language === 'en' ? 'en' : 'zh-CN';
  document.querySelectorAll('[data-en]').forEach((element) => {
    element.innerHTML = language === 'en' ? element.dataset.en : originalText.get(element);
  });
  languageButtons.forEach((button) => {
    button.textContent = language === 'en' ? 'EN / 中' : '中 / EN';
    button.setAttribute('aria-label', language === 'en' ? '切换到中文' : 'Switch to English');
  });
  localStorage.setItem('modu-language', language);
}

applyLanguage(localStorage.getItem('modu-language') === 'en' ? 'en' : 'zh');

languageButtons.forEach((button) => {
  button.addEventListener('click', () => {
    applyLanguage(root.lang === 'en' ? 'zh' : 'en');
  });
});

const docsSearch = document.querySelector('.docs-search input');
if (docsSearch) {
  const navLinks = [...document.querySelectorAll('.docs-nav a')];
  const navGroups = [...document.querySelectorAll('.docs-nav > div')];
  const emptyState = document.querySelector('.search-empty');

  docsSearch.addEventListener('input', () => {
    const query = docsSearch.value.trim().toLocaleLowerCase();
    let visibleCount = 0;
    navLinks.forEach((link) => {
      const visible = !query || link.textContent.toLocaleLowerCase().includes(query);
      link.hidden = !visible;
      if (visible) visibleCount += 1;
    });
    navGroups.forEach((group) => {
      group.hidden = ![...group.querySelectorAll('a')].some((link) => !link.hidden);
    });
    if (emptyState) emptyState.hidden = visibleCount !== 0;
  });

  document.addEventListener('keydown', (event) => {
    if ((event.metaKey || event.ctrlKey) && event.key.toLocaleLowerCase() === 'k') {
      event.preventDefault();
      docsSearch.focus();
    }
  });
}

const articleSections = [...document.querySelectorAll('.docs-article section[id], .docs-article header[id]')];
const tocLinks = [...document.querySelectorAll('.docs-toc a')];

if (articleSections.length && tocLinks.length) {
  const sectionObserver = new IntersectionObserver(
    (entries) => {
      const visible = entries
        .filter((entry) => entry.isIntersecting)
        .sort((a, b) => a.boundingClientRect.top - b.boundingClientRect.top)[0];
      if (!visible) return;
      tocLinks.forEach((link) => {
        link.classList.toggle('active', link.getAttribute('href') === `#${visible.target.id}`);
      });
    },
    { rootMargin: '-20% 0px -70%', threshold: 0 }
  );
  articleSections.forEach((section) => sectionObserver.observe(section));
}

const legacyAnimatedElements = document.querySelectorAll('.fade-up');
if (legacyAnimatedElements.length) {
  const legacyObserver = new IntersectionObserver(
    (entries, observer) => {
      entries.forEach((entry) => {
        if (!entry.isIntersecting) return;
        entry.target.classList.add('visible');
        observer.unobserve(entry.target);
      });
    },
    { threshold: 0.08 }
  );
  legacyAnimatedElements.forEach((element) => legacyObserver.observe(element));
}
