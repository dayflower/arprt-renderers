// This script owns geometry only. It never evaluates configuration expressions,
// fetches assets, or interpolates input into executable source.
(async () => {
  await document.fonts.ready;
  await new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)));
  const fail = () => { document.body.dataset.layoutError = 'overflow'; };
  for (const element of document.querySelectorAll('.number.auto-size')) {
    let size = 64;
    while (size > 40 && element.firstElementChild.scrollWidth > element.clientWidth) {
      element.style.fontSize = --size + 'px';
    }
  }
  for (const svg of document.querySelectorAll('.chart')) {
    const labels = [...svg.querySelectorAll('.direct-label')];
    const height = svg.clientHeight - 60;
    // SVG text x/y are coordinate lists and do not accept CSS calc(). Keep text
    // in CSS pixels while only the nested data plot uses normalized coordinates.
    for (const element of svg.querySelectorAll('text')) {
      if (element.hasAttribute('data-xfactor')) {
        element.setAttribute('x', 48 + (svg.clientWidth - 96) * Number(element.dataset.xfactor));
      }
      if (element.hasAttribute('data-yfactor')) {
        element.setAttribute('y', 18 + height * Number(element.dataset.yfactor));
      }
      if (element.hasAttribute('data-bottom')) {
        element.setAttribute('y', svg.clientHeight - Number(element.dataset.bottom));
      }
    }
    if (labels.length * 16 > height) {
      fail();
      return;
    }
    let last = -16;
    for (let i = 0; i < labels.length; i++) {
      const element = labels[i];
      element.setAttribute('x', svg.clientWidth - 44);
      const y = Math.max(last + 16, Math.min(height - 8 - (labels.length - 1 - i) * 16,
        Number(element.dataset.y) * height));
      element.setAttribute('y', 18 + y);
      last = y;
    }
    // Omit colliding tick labels, preserving a partial bucket's explicit mark.
    // Bars and missing slots themselves are never removed.
    let previous = null;
    for (const element of svg.querySelectorAll('.x-label[data-bucket]')) {
      if (previous && element.getBoundingClientRect().left < previous.getBoundingClientRect().right + 8) {
        if (element.textContent.endsWith('*')) {
          previous.remove();
          previous = element;
        } else {
          element.remove();
        }
      } else {
        previous = element;
      }
    }
    let right = -Infinity;
    for (const element of svg.querySelectorAll('.x-label:not([data-bucket])')) {
      const bounds = element.getBoundingClientRect();
      if (bounds.top < svg.getBoundingClientRect().bottom - 20 && bounds.left < right + 8) {
        element.remove();
      } else {
        right = bounds.right;
      }
    }
  }
  for (const element of document.querySelectorAll('svg text[data-max]')) {
    const limit = Number(element.dataset.max);
    const full = element.textContent;
    let text = full;
    while (element.getComputedTextLength() > limit && text.length) {
      text = Array.from(text).slice(0, -1).join('');
      element.textContent = text + '…';
    }
    if (text !== full) element.dataset.truncated = 'true';
  }
  let ok = true;
  for (const element of document.querySelectorAll('.panel,.chart,.text,.number,h2,.status')) {
    const bounds = element.getBoundingClientRect();
    const parent = element.parentElement.getBoundingClientRect();
    if (bounds.width < 1 || bounds.height < 1 || bounds.left < parent.left - 1 ||
        bounds.right > parent.right + 1 || bounds.top < parent.top - 1 || bounds.bottom > parent.bottom + 1) {
      ok = false;
    }
  }
  for (const element of document.querySelectorAll('.chart')) {
    if (element.clientWidth < 160 || element.clientHeight < 156) ok = false;
  }
  for (const element of document.querySelectorAll('.text')) {
    if (element.scrollHeight > element.clientHeight + 1) ok = false;
  }
  for (const element of document.querySelectorAll('svg text')) {
    const bounds = element.getBoundingClientRect();
    const parent = element.closest('.chart').getBoundingClientRect();
    if (bounds.left < parent.left - 1 || bounds.right > parent.right + 1 ||
        bounds.top < parent.top - 1 || bounds.bottom > parent.bottom + 1) ok = false;
  }
  document.body.dataset.geometry = ok ? 'ok' : 'overflow';
  if (!ok) {
    fail();
    return;
  }
  const ready=document.createElement('span');
  ready.id = 'prometheus-ready';
  document.body.appendChild(ready);
})().catch(() => { document.body.dataset.layoutError = 'geometry'; });
