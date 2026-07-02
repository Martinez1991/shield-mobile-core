import java.nio.file.*;
import java.security.MessageDigest;
import java.util.Arrays;
import java.util.Base64;
import javax.crypto.Cipher;
import javax.crypto.spec.GCMParameterSpec;
import javax.crypto.spec.SecretKeySpec;

// Mirrors the injected smali Lshield/rt/SH;->d line-by-line: unmask embedded
// material with the per-build XOR keystream, key = SHA-256(material), then
// AES-256-GCM decrypt of nonce(12) || ciphertext || tag(16). Running it on
// Go-produced vectors validates the full decryptor path on the JVM (same
// int/crypto semantics as Android ART). Used by scripts/validate-aes.sh.
// Input file: line1=maskedMaterial(hex) line2="seed8 step" line3=blob(base64) line4=plaintext
public class AesDec {
  static byte[] hex(String s){ byte[] b=new byte[s.length()/2];
    for(int i=0;i<b.length;i++) b[i]=(byte)Integer.parseInt(s.substring(2*i,2*i+2),16); return b; }

  public static void main(String[] a) throws Exception {
    var lines = Files.readAllLines(Paths.get(a[0]));
    byte[] material = hex(lines.get(0));
    String[] ks = lines.get(1).trim().split("\\s+");
    int seed8 = Integer.parseInt(ks[0]), step = Integer.parseInt(ks[1]);
    byte[] blob = Base64.getDecoder().decode(lines.get(2));
    String expected = lines.get(3);

    // unmask (mirrors the smali :umloop)
    for (int i = 0; i < material.length; i++) {
      material[i] = (byte)(material[i] ^ ((seed8 + i*step) & 0xff));
    }
    byte[] key = MessageDigest.getInstance("SHA-256").digest(material);
    byte[] iv = Arrays.copyOfRange(blob, 0, 12);
    byte[] body = Arrays.copyOfRange(blob, 12, blob.length);
    Cipher c = Cipher.getInstance("AES/GCM/NoPadding");
    c.init(Cipher.DECRYPT_MODE, new SecretKeySpec(key, "AES"), new GCMParameterSpec(128, iv));
    String got = new String(c.doFinal(body), "UTF-8");
    boolean ok = got.equals(expected);
    System.out.println("decrypt=" + got + " expected=" + expected + (ok ? " OK" : " MISMATCH"));
    System.out.println(ok ? "AES-JVM-VALIDATION OK" : "AES-JVM-VALIDATION FAILED");
    if (!ok) System.exit(1);
  }
}
