import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import { channelFormSchema } from './channel-form'

function makeChannelForm(proxy: string) {
  return {
    name: 'proxy-test',
    type: 1,
    key: 'sk-test',
    models: 'gpt-test',
    group: ['default'],
    status: 1,
    proxy,
  }
}

describe('channel proxy validation', () => {
  test('accepts supported proxy protocols and an optional root path', () => {
    for (const proxy of [
      '',
      'http://proxy.example:8080',
      'https://proxy.example:8443/',
      'socks5://proxy.example',
      'socks5h://user:pass@proxy.example:1080',
    ]) {
      assert.equal(
        channelFormSchema.safeParse(makeChannelForm(proxy)).success,
        true
      )
    }
  })

  test('rejects unsupported protocols and URL suffixes', () => {
    for (const proxy of [
      'ftp://proxy.example',
      'proxy.example:8080',
      'http://proxy.example/path',
      'http://proxy.example?x=1',
      'http://proxy.example#fragment',
      'http://proxy.example:0',
    ]) {
      assert.equal(
        channelFormSchema.safeParse(makeChannelForm(proxy)).success,
        false
      )
    }
  })
})
