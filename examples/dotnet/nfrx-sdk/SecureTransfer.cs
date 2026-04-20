using System;
using System.Collections.Generic;
using System.Security.Cryptography;
using System.Security.Cryptography.Pkcs;
using System.Security.Cryptography.X509Certificates;
using System.Text;

namespace NfrxSdk;

public sealed class SecureIdentity : IDisposable
{
    public SecureIdentity(X509Certificate2 certificate, string certificatePem)
    {
        Certificate = certificate;
        CertificatePem = certificatePem;
    }

    public X509Certificate2 Certificate { get; }
    public string CertificatePem { get; }

    public void Dispose()
    {
        Certificate.Dispose();
    }
}

public sealed class HeaderBodyEnvelope
{
    public HeaderBodyEnvelope(Dictionary<string, string> headers, byte[] body)
    {
        Headers = headers;
        Body = body;
    }

    public Dictionary<string, string> Headers { get; }
    public byte[] Body { get; }
}

public static class SecureTransfer
{
    public const string Scheme = "cms-x509-selfsigned-v1";
    public const string Envelope = "header-body-v1";
    public const string CmsContentType = "application/pkcs7-mime";
    private const string Aes256CbcOid = "2.16.840.1.101.3.4.1.42";

    public static SecureIdentity GenerateSelfSignedIdentity(string commonName)
    {
        using var rsa = RSA.Create(2048);
        var subject = new X500DistinguishedName($"CN={commonName}, O=nfrx example");
        var request = new CertificateRequest(subject, rsa, HashAlgorithmName.SHA256, RSASignaturePadding.Pkcs1);
        request.CertificateExtensions.Add(new X509BasicConstraintsExtension(false, false, 0, true));
        request.CertificateExtensions.Add(new X509SubjectKeyIdentifierExtension(request.PublicKey, false));
        using var generated = request.CreateSelfSigned(DateTimeOffset.UtcNow.AddMinutes(-5), DateTimeOffset.UtcNow.AddDays(7));
        var cert = new X509Certificate2(generated.Export(X509ContentType.Pfx), (string?)null, X509KeyStorageFlags.Exportable);
        return new SecureIdentity(cert, cert.ExportCertificatePem());
    }

    public static byte[] EncryptCms(byte[] plaintext, string recipientCertificatePem)
    {
        using var recipient = X509Certificate2.CreateFromPem(recipientCertificatePem);
        var cms = new EnvelopedCms(new ContentInfo(plaintext), new AlgorithmIdentifier(new Oid(Aes256CbcOid)));
        cms.Encrypt(new CmsRecipient(SubjectIdentifierType.IssuerAndSerialNumber, recipient));
        return cms.Encode();
    }

    public static byte[] DecryptCms(byte[] ciphertext, X509Certificate2 recipientCertificate)
    {
        var cms = new EnvelopedCms();
        cms.Decode(ciphertext);
        cms.Decrypt(new X509Certificate2Collection(recipientCertificate));
        return cms.ContentInfo.Content;
    }

    public static byte[] BuildHeaderBodyEnvelope(
        string contentType,
        byte[] body,
        string role,
        IReadOnlyDictionary<string, string>? extraHeaders = null)
    {
        var headers = new Dictionary<string, string>(StringComparer.OrdinalIgnoreCase)
        {
            ["X-Nfrx-Envelope-Version"] = "1",
            ["Content-Type"] = contentType,
            ["Content-Length"] = body.Length.ToString(),
            ["X-Example-Protocol"] = Scheme,
            ["X-Example-Role"] = role,
        };
        if (extraHeaders != null)
        {
            foreach (var pair in extraHeaders)
            {
                if (string.IsNullOrWhiteSpace(pair.Key) || pair.Key.Contains('\r') || pair.Key.Contains('\n'))
                {
                    throw new InvalidOperationException($"Invalid header name: {pair.Key}");
                }
                if (pair.Value.Contains('\r') || pair.Value.Contains('\n'))
                {
                    throw new InvalidOperationException($"Invalid header value for {pair.Key}");
                }
                headers[pair.Key] = pair.Value;
            }
        }

        var builder = new StringBuilder();
        foreach (var pair in headers)
        {
            builder.Append(pair.Key).Append(": ").Append(pair.Value).Append("\r\n");
        }
        builder.Append("\r\n");

        var headerBytes = Encoding.UTF8.GetBytes(builder.ToString());
        var envelope = new byte[headerBytes.Length + body.Length];
        Buffer.BlockCopy(headerBytes, 0, envelope, 0, headerBytes.Length);
        Buffer.BlockCopy(body, 0, envelope, headerBytes.Length, body.Length);
        return envelope;
    }

    public static HeaderBodyEnvelope ParseHeaderBodyEnvelope(byte[] envelope)
    {
        var separatorIndex = FindSeparator(envelope);
        if (separatorIndex < 0)
        {
            throw new InvalidOperationException("Missing CRLFCRLF envelope separator.");
        }

        var headerText = Encoding.UTF8.GetString(envelope, 0, separatorIndex);
        var bodyOffset = separatorIndex + 4;
        var body = new byte[envelope.Length - bodyOffset];
        Buffer.BlockCopy(envelope, bodyOffset, body, 0, body.Length);

        var headers = new Dictionary<string, string>(StringComparer.OrdinalIgnoreCase);
        foreach (var line in headerText.Split("\r\n", StringSplitOptions.None))
        {
            if (line.Length == 0)
            {
                throw new InvalidOperationException("Unexpected empty header line.");
            }
            if (line[0] == ' ' || line[0] == '\t')
            {
                throw new InvalidOperationException("Folded headers are not supported.");
            }
            var idx = line.IndexOf(':');
            if (idx <= 0)
            {
                throw new InvalidOperationException($"Malformed header line: {line}");
            }
            var key = line[..idx].Trim();
            var value = line[(idx + 1)..].Trim();
            if (key.Length == 0)
            {
                throw new InvalidOperationException("Header name cannot be empty.");
            }
            if (!headers.TryAdd(key, value))
            {
                throw new InvalidOperationException($"Duplicate header: {key}");
            }
        }

        foreach (var required in new[] { "X-Nfrx-Envelope-Version", "Content-Type", "Content-Length" })
        {
            if (!headers.ContainsKey(required))
            {
                throw new InvalidOperationException($"Missing required header: {required}");
            }
        }

        if (!int.TryParse(headers["Content-Length"], out var expectedLength) || expectedLength < 0)
        {
            throw new InvalidOperationException("Invalid Content-Length header.");
        }
        if (expectedLength != body.Length)
        {
            throw new InvalidOperationException($"Content-Length mismatch: expected {expectedLength}, got {body.Length}.");
        }

        return new HeaderBodyEnvelope(headers, body);
    }

    private static int FindSeparator(byte[] envelope)
    {
        for (var i = 0; i <= envelope.Length - 4; i++)
        {
            if (envelope[i] == '\r' && envelope[i + 1] == '\n' && envelope[i + 2] == '\r' && envelope[i + 3] == '\n')
            {
                return i;
            }
        }
        return -1;
    }
}
