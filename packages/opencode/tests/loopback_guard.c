#define _GNU_SOURCE
#include <arpa/inet.h>
#include <errno.h>
#include <fcntl.h>
#include <linux/audit.h>
#include <linux/filter.h>
#include <linux/seccomp.h>
#include <poll.h>
#include <signal.h>
#include <stddef.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/ioctl.h>
#include <sys/prctl.h>
#include <sys/socket.h>
#include <sys/stat.h>
#include <sys/syscall.h>
#include <sys/uio.h>
#include <sys/wait.h>
#include <unistd.h>

struct held_socket {
    int descriptor;
    ino_t inode;
};

static struct held_socket sockets[4096];
static unsigned allowed_connects, allowed_binds, denied_operations;

static void fail(const char *operation) {
    perror(operation);
    exit(125);
}

static void pass_descriptor(int channel, int descriptor) {
    char value = 0;
    struct iovec vector = {.iov_base = &value, .iov_len = 1};
    char control[CMSG_SPACE(sizeof(int))] = {0};
    struct msghdr message = {.msg_iov = &vector, .msg_iovlen = 1,
                            .msg_control = control, .msg_controllen = sizeof(control)};
    struct cmsghdr *header = CMSG_FIRSTHDR(&message);
    header->cmsg_level = SOL_SOCKET;
    header->cmsg_type = SCM_RIGHTS;
    header->cmsg_len = CMSG_LEN(sizeof(int));
    memcpy(CMSG_DATA(header), &descriptor, sizeof(descriptor));
    if (sendmsg(channel, &message, 0) != 1) fail("send listener");
}

static int receive_descriptor(int channel) {
    char value;
    struct iovec vector = {.iov_base = &value, .iov_len = 1};
    char control[CMSG_SPACE(sizeof(int))] = {0};
    struct msghdr message = {.msg_iov = &vector, .msg_iovlen = 1,
                            .msg_control = control, .msg_controllen = sizeof(control)};
    if (recvmsg(channel, &message, MSG_CMSG_CLOEXEC) != 1) fail("receive listener");
    struct cmsghdr *header = CMSG_FIRSTHDR(&message);
    if (!header || header->cmsg_level != SOL_SOCKET || header->cmsg_type != SCM_RIGHTS)
        fail("missing listener");
    int descriptor;
    memcpy(&descriptor, CMSG_DATA(header), sizeof(descriptor));
    return descriptor;
}

static void filter_child(void) {
    struct sock_filter instructions[] = {
        BPF_STMT(BPF_LD | BPF_W | BPF_ABS, offsetof(struct seccomp_data, arch)),
        BPF_JUMP(BPF_JMP | BPF_JEQ | BPF_K, AUDIT_ARCH_X86_64, 1, 0),
        BPF_STMT(BPF_RET | BPF_K, SECCOMP_RET_KILL_PROCESS),
        BPF_STMT(BPF_LD | BPF_W | BPF_ABS, offsetof(struct seccomp_data, nr)),
        BPF_JUMP(BPF_JMP | BPF_JSET | BPF_K, 0x40000000, 0, 1),
        BPF_STMT(BPF_RET | BPF_K, SECCOMP_RET_KILL_PROCESS),
        BPF_JUMP(BPF_JMP | BPF_JEQ | BPF_K, SYS_socket, 5, 0),
        BPF_JUMP(BPF_JMP | BPF_JEQ | BPF_K, SYS_connect, 4, 0),
        BPF_JUMP(BPF_JMP | BPF_JEQ | BPF_K, SYS_bind, 3, 0),
        BPF_JUMP(BPF_JMP | BPF_JEQ | BPF_K, SYS_close, 2, 0),
        BPF_JUMP(BPF_JMP | BPF_JEQ | BPF_K, SYS_io_uring_setup, 2, 0),
        BPF_STMT(BPF_RET | BPF_K, SECCOMP_RET_ALLOW),
        BPF_STMT(BPF_RET | BPF_K, SECCOMP_RET_USER_NOTIF),
        BPF_STMT(BPF_RET | BPF_K, SECCOMP_RET_ERRNO | EPERM),
    };
    struct sock_fprog program = {.len = sizeof(instructions) / sizeof(instructions[0]), .filter = instructions};
    if (prctl(PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0)) fail("no new privileges");
    int listener = syscall(SYS_seccomp, SECCOMP_SET_MODE_FILTER, SECCOMP_FILTER_FLAG_NEW_LISTENER, &program);
    if (listener < 0) fail("seccomp listener");
    pass_descriptor(3, listener);
}

static struct held_socket *find_socket(pid_t process, int descriptor) {
    char path[80];
    struct stat status;
    snprintf(path, sizeof(path), "/proc/%d/fd/%d", process, descriptor);
    if (stat(path, &status) || !S_ISSOCK(status.st_mode)) return NULL;
    for (size_t index = 0; index < sizeof(sockets) / sizeof(sockets[0]); index++)
        if (sockets[index].inode == status.st_ino) return &sockets[index];
    return NULL;
}

static void release_socket(struct held_socket *held) {
    close(held->descriptor);
    held->inode = 0;
}

static void handle(int listener, const struct seccomp_notif *request) {
    struct seccomp_notif_resp response = {.id = request->id};
    int descriptor = (int)request->data.args[0];
    if (request->data.nr == SYS_socket) {
        int domain = descriptor;
        int type = (int)request->data.args[1];
        int protocol = (int)request->data.args[2];
        if (domain == AF_UNIX) {
            response.flags = SECCOMP_USER_NOTIF_FLAG_CONTINUE;
        } else if (domain != AF_INET || (type & ~(SOCK_CLOEXEC | SOCK_NONBLOCK)) != SOCK_STREAM ||
                   (protocol != 0 && protocol != IPPROTO_TCP)) {
            response.error = -EPERM;
            denied_operations++;
        } else {
            struct held_socket *held = NULL;
            for (size_t index = 0; index < sizeof(sockets) / sizeof(sockets[0]); index++)
                if (!sockets[index].inode) { held = &sockets[index]; break; }
            if (!held) fail("socket budget");
            int local = socket(domain, type, protocol);
            if (local < 0) {
                response.error = -errno;
            } else {
                struct stat status;
                if (fstat(local, &status)) fail("socket identity");
                held->descriptor = local;
                held->inode = status.st_ino;
                struct seccomp_notif_addfd addition = {
                    .id = request->id, .flags = SECCOMP_ADDFD_FLAG_SEND,
                    .srcfd = local, .newfd_flags = (type & SOCK_CLOEXEC) ? O_CLOEXEC : 0,
                };
                if (ioctl(listener, SECCOMP_IOCTL_NOTIF_ADDFD, &addition) >= 0) return;
                int failure = errno;
                release_socket(held);
                if (failure == ENOENT) return;
                errno = failure;
                fail("inject socket");
            }
        }
    } else {
        struct held_socket *held = find_socket(request->pid, descriptor);
        if (request->data.nr == SYS_close) {
            if (held) release_socket(held);
            response.flags = SECCOMP_USER_NOTIF_FLAG_CONTINUE;
        } else {
            struct sockaddr_in address = {0};
            struct iovec local = {.iov_base = &address, .iov_len = sizeof(address)};
            struct iovec remote = {.iov_base = (void *)(uintptr_t)request->data.args[1], .iov_len = sizeof(address)};
            if (!held || request->data.args[2] != sizeof(address) ||
                process_vm_readv(request->pid, &local, 1, &remote, 1, 0) != sizeof(address) ||
                address.sin_family != AF_INET || address.sin_addr.s_addr != htonl(INADDR_LOOPBACK)) {
                response.error = -EPERM;
                denied_operations++;
            } else if (ioctl(listener, SECCOMP_IOCTL_NOTIF_ID_VALID, &request->id) == 0) {
                int result;
                if (request->data.nr == SYS_connect) {
                    result = connect(held->descriptor, (struct sockaddr *)&address, sizeof(address));
                    allowed_connects++;
                } else {
                    result = bind(held->descriptor, (struct sockaddr *)&address, sizeof(address));
                    allowed_binds++;
                }
                response.error = result < 0 ? -errno : 0;
            } else {
                return;
            }
            if (held) release_socket(held);
        }
    }
    if (ioctl(listener, SECCOMP_IOCTL_NOTIF_SEND, &response) < 0 && errno != ENOENT)
        fail("respond to socket operation");
}

int main(int count, char **arguments) {
    if (count < 4) {
        fprintf(stderr, "usage: loopback-guard report.json command arguments...\n");
        return 125;
    }
    int channels[2];
    if (socketpair(AF_UNIX, SOCK_SEQPACKET | SOCK_CLOEXEC, 0, channels)) fail("control channel");
    pid_t child = fork();
    if (child < 0) fail("fork fixture");
    if (child == 0) {
        close(channels[0]);
        if (channels[1] != 3 && dup2(channels[1], 3) < 0) fail("child control channel");
        filter_child();
        close(3);
        if (setenv("MAGPIE_OPENCODE_LOOPBACK_GUARD", "seccomp-broker", 1)) fail("guard marker");
        execvp(arguments[2], arguments + 2);
        fail("execute fixture");
    }
    close(channels[1]);
    int listener = receive_descriptor(channels[0]);
    close(channels[0]);
    int status = 0;
    for (;;) {
        struct pollfd ready = {.fd = listener, .events = POLLIN};
        int result = poll(&ready, 1, 50);
        if (result < 0 && errno != EINTR) fail("poll notification");
        if (result > 0 && (ready.revents & POLLIN)) {
            struct seccomp_notif request = {0};
            if (ioctl(listener, SECCOMP_IOCTL_NOTIF_RECV, &request) == 0) handle(listener, &request);
            else if (errno != ENOENT && errno != EINTR) fail("receive notification");
        }
        pid_t finished = waitpid(child, &status, WNOHANG);
        if (finished == child) break;
        if (finished < 0) fail("join fixture");
    }
    close(listener);
    for (size_t index = 0; index < sizeof(sockets) / sizeof(sockets[0]); index++)
        if (sockets[index].inode) release_socket(&sockets[index]);
    FILE *report = fopen(arguments[1], "w");
    if (!report) fail("guard report");
    struct stat root;
    if (stat("/", &root)) fail("root ownership");
    fprintf(report, "{\"guard\":\"seccomp-broker\",\"connects\":%u,\"binds\":%u,\"denied\":%u,"
                    "\"networkPointerContinuation\":false,\"rootOwnerUID\":%u,\"childExit\":%d}\n",
            allowed_connects, allowed_binds, denied_operations, (unsigned)root.st_uid,
            WIFEXITED(status) ? WEXITSTATUS(status) : 128 + WTERMSIG(status));
    if (fclose(report)) fail("close guard report");
    return WIFEXITED(status) ? WEXITSTATUS(status) : 128 + WTERMSIG(status);
}
