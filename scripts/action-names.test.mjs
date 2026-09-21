import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { createActionNames } from '../ui/app/action-names.mjs';

test('rotation screenshot actions have readable names, including consumables outside the gear pool', () => {
  const db = JSON.parse(readFileSync(new URL('../assets/database/db.json', import.meta.url)));
  const resolve = createActionNames({}, db);
  for (const [itemId, name] of [[10646, 'Goblin Sapper Charge'], [18641, 'Dense Dynamite']])
    assert.equal(resolve({ oneofKind: 'itemId', itemId }), name);
  for (const [spellId, name] of [[20572, 'Blood Fury'], [24427, 'Diamond Flask'], [29602, 'Jom Gabbar']])
    assert.equal(resolve({ oneofKind: 'spellId', spellId }), name);
});

test('item and spell namespaces stay separate; missing names are not guessed', () => {
  const resolve = createActionNames({ 7: 'Example spell' }, { items: [{ id: 7, name: 'Example item' }] });
  assert.equal(resolve({ oneofKind: 'spellId', spellId: 7 }), 'Example spell');
  assert.equal(resolve({ oneofKind: 'itemId', itemId: 7 }), 'Example item');
  assert.match(resolve({ oneofKind: 'spellId', spellId: 8 }), /Unidentified ability/);
  assert.equal(resolve({ oneofKind: 'otherId', otherId: 7 }), undefined);
});
