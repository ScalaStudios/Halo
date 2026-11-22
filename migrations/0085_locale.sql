alter table users add column locale text check (locale in ('en', 'es'));
