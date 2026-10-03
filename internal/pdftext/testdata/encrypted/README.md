# Encrypted PDFs

Used by internal/pdfsmart's decryption tests. User password `user-secret`, owner password `owner-secret`.

- `aes256_*`, `aes256r5_nouser`, `aes128_nouser`, `rc4_128_nouser`: the 3-page `../three_pages.pdf` encrypted by
  [pypdf](https://pypdf.readthedocs.io) (`PdfWriter.encrypt`), a producer independent of the readers under test.
  `_nouser` files have an empty user password (the owner password only restricts permissions), the common case of
  an emailed bank statement.
- `mupdf_aes256_nouser.pdf`, `mupdf_aes256_user.pdf`: a one-page "Encrypted Hello" document encrypted by MuPDF
  (`mutool clean -E aes-256`), copied from https://github.com/razvandimescu/gopdf (MIT, Copyright (c) Razvan
  Dimescu). MuPDF writes the `/Encrypt` dictionary inline in the trailer, which some readers do not expect.
