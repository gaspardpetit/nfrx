"""CMS/X.509 secure transfer helpers for nfrx example jobs flows."""

from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime, timedelta, timezone
from typing import Mapping, Optional

from cryptography import x509
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import rsa
from cryptography.hazmat.primitives.ciphers import algorithms
from cryptography.hazmat.primitives.serialization import pkcs7
from cryptography.x509.oid import NameOID

SCHEME = "cms-x509-selfsigned-v1"
ENVELOPE = "header-body-v1"
CMS_CONTENT_TYPE = "application/pkcs7-mime"


@dataclass
class SecureIdentity:
    certificate: x509.Certificate
    private_key: rsa.RSAPrivateKey
    certificate_pem: str


def generate_self_signed_identity(common_name: str) -> SecureIdentity:
    key = rsa.generate_private_key(public_exponent=65537, key_size=2048)
    subject = issuer = x509.Name(
        [
            x509.NameAttribute(NameOID.COMMON_NAME, common_name),
            x509.NameAttribute(NameOID.ORGANIZATION_NAME, "nfrx example"),
        ]
    )
    now = datetime.now(timezone.utc)
    cert = (
        x509.CertificateBuilder()
        .subject_name(subject)
        .issuer_name(issuer)
        .public_key(key.public_key())
        .serial_number(x509.random_serial_number())
        .not_valid_before(now - timedelta(minutes=5))
        .not_valid_after(now + timedelta(days=7))
        .add_extension(x509.BasicConstraints(ca=False, path_length=None), critical=True)
        .sign(key, hashes.SHA256())
    )
    cert_pem = cert.public_bytes(serialization.Encoding.PEM).decode("ascii")
    return SecureIdentity(certificate=cert, private_key=key, certificate_pem=cert_pem)


def build_header_body_envelope(
    content_type: str,
    body: bytes,
    role: str,
    extra_headers: Optional[Mapping[str, str]] = None,
) -> bytes:
    headers: dict[str, str] = {
        "X-Nfrx-Envelope-Version": "1",
        "Content-Type": content_type,
        "Content-Length": str(len(body)),
        "X-Example-Protocol": SCHEME,
        "X-Example-Role": role,
    }
    if extra_headers:
        for key, value in extra_headers.items():
            if not key or "\r" in key or "\n" in key:
                raise ValueError(f"invalid header name: {key!r}")
            if "\r" in value or "\n" in value:
                raise ValueError(f"invalid header value for {key!r}")
            headers[key] = value
    header_blob = "".join(f"{key}: {value}\r\n" for key, value in headers.items()).encode("utf-8")
    return header_blob + b"\r\n" + body


def parse_header_body_envelope(data: bytes) -> tuple[dict[str, str], bytes]:
    marker = b"\r\n\r\n"
    idx = data.find(marker)
    if idx < 0:
        raise ValueError("missing CRLFCRLF envelope separator")

    header_blob = data[:idx].decode("utf-8")
    body = data[idx + len(marker) :]
    headers: dict[str, str] = {}
    for raw_line in header_blob.split("\r\n"):
        if raw_line == "":
            raise ValueError("unexpected empty header line")
        if raw_line[0] in {" ", "\t"}:
            raise ValueError("folded headers are not supported")
        if ":" not in raw_line:
            raise ValueError(f"malformed header line: {raw_line!r}")
        key, value = raw_line.split(":", 1)
        key = key.strip()
        value = value.strip()
        if not key:
            raise ValueError("header name cannot be empty")
        canonical = key.lower()
        if canonical in headers:
            raise ValueError(f"duplicate header: {key}")
        headers[canonical] = value

    for required in ("x-nfrx-envelope-version", "content-type", "content-length"):
        if required not in headers:
            raise ValueError(f"missing required header: {required}")

    try:
        content_length = int(headers["content-length"])
    except ValueError as exc:
        raise ValueError("invalid Content-Length header") from exc
    if content_length != len(body):
        raise ValueError(f"Content-Length mismatch: expected {content_length}, got {len(body)}")
    return headers, body


def encrypt_cms_der(plaintext: bytes, recipient_cert_pem: str) -> bytes:
    cert = x509.load_pem_x509_certificate(recipient_cert_pem.encode("ascii"))
    return (
        pkcs7.PKCS7EnvelopeBuilder()
        .set_data(plaintext)
        .set_content_encryption_algorithm(algorithms.AES256)
        .add_recipient(cert)
        .encrypt(serialization.Encoding.DER, [])
    )


def decrypt_cms_der(ciphertext: bytes, identity: SecureIdentity) -> bytes:
    return pkcs7.pkcs7_decrypt_der(ciphertext, identity.certificate, identity.private_key, [])
