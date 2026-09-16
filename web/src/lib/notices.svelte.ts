export interface Notice {
  id: number;
  tone: 'ok' | 'fail';
  text: string;
}

let next = 1;

class Notices {
  items = $state<Notice[]>([]);

  push(tone: Notice['tone'], text: string) {
    const id = next++;
    this.items = [...this.items.slice(-3), { id, tone, text }];
    setTimeout(() => this.dismiss(id), tone === 'fail' ? 12000 : 5000);
  }

  dismiss(id: number) {
    this.items = this.items.filter((n) => n.id !== id);
  }
}

export const notices = new Notices();
