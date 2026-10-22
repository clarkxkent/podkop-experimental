// onMount resolves with the element of the id once it is in the page and visible. One
// IntersectionObserver watches the element (a new one only when LuCI renders a new element
// of the id); both observers go once it is visible. With previous, only another element of
// the id counts: LuCI renders the form again while the old one is still in the page.
export async function onMount(
  id: string,
  previous?: HTMLElement | null,
): Promise<HTMLElement> {
  return new Promise((resolve) => {
    let watched: HTMLElement | null = null;
    let visibility: IntersectionObserver | undefined;

    const mutations = new MutationObserver(() => check());

    function done(element: HTMLElement) {
      mutations.disconnect();
      visibility?.disconnect();
      resolve(element);
    }

    function check() {
      const element = document.getElementById(id);
      if (!element || element === previous) return;
      if (element.offsetParent !== null) {
        done(element);
        return;
      }
      if (element === watched) return;
      visibility?.disconnect();
      watched = element;
      visibility = new IntersectionObserver((entries) => {
        if (entries.some((entry) => entry.isIntersecting)) done(element);
      });
      visibility.observe(element);
    }

    mutations.observe(document.body, { childList: true, subtree: true });
    check();
  });
}
