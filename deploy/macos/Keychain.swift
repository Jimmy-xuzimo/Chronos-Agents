import Foundation
import Security
guard CommandLine.arguments.count == 3 else { exit(2) }
let action = CommandLine.arguments[1]
let account = CommandLine.arguments[2]
let query: [String:Any] = [kSecClass as String:kSecClassGenericPassword, kSecAttrService as String:"dev.chronos.agents", kSecAttrAccount as String:account]
switch action {
case "save":
 let input = FileHandle.standardInput.readDataToEndOfFile()
 guard !input.isEmpty && input.count < 4096 else { exit(2) }
 let status = SecItemUpdate(query as CFDictionary, [kSecValueData as String:input] as CFDictionary)
 if status == errSecItemNotFound {
  var q = query; q[kSecValueData as String] = input
  q[kSecAttrAccessible as String] = kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly
  guard SecItemAdd(q as CFDictionary,nil) == errSecSuccess else { exit(1) }
 } else if status != errSecSuccess { exit(1) }
case "read":
 var q = query; q[kSecReturnData as String] = true; q[kSecMatchLimit as String] = kSecMatchLimitOne
 var item: CFTypeRef?
 guard SecItemCopyMatching(q as CFDictionary,&item) == errSecSuccess, let data = item as? Data else { exit(1) }
 FileHandle.standardOutput.write(data)
case "delete":
 let status = SecItemDelete(query as CFDictionary)
 if status != errSecSuccess && status != errSecItemNotFound { exit(1) }
default: exit(2)
}
